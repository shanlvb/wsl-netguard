package render

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

// TestFakeIPProtocol uses only loopback sockets and temporary files. The mixed
// inbound substitutes for TUN delivery; host routes and firewall are untouched.
// Run with WSL_NETGUARD_SING_BOX=/path/to/sing-box go test ./internal/render -run TestFakeIPProtocol -v.
func TestFakeIPProtocol(t *testing.T) {
	binary := fakeIPBinary(t)
	upstream, requests := fakeIPMockSOCKS(t)
	cfg := config.Default()
	cfg.SocksPort = upstream.Addr().(*net.TCPAddr).Port
	data, err := SingBoxJSON(cfg, netip.MustParseAddr("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	dns := doc["dns"].(map[string]any)
	hasFakeIP := false
	for i, server := range dns["servers"].([]any) {
		entry := server.(map[string]any)
		if entry["type"] == "fakeip" {
			hasFakeIP = true
		}
		if entry["tag"] == "remote-doh" {
			// Keep the generated DNS rule selection and proxy detour; substitute
			// a deterministic DNS-over-TCP responder for public DoH.
			dns["servers"].([]any)[i] = map[string]any{"type": "tcp", "tag": "remote-doh", "server": "203.0.113.53", "server_port": 53, "detour": "proxy"}
		}
	}
	if !hasFakeIP {
		t.Fatal("generated configuration has no FakeIP DNS server")
	}
	path, address := fakeIPLoopbackConfig(t, doc)
	stop := startFakeIPBox(t, binary, path, address)

	var firstIP netip.Addr
	for _, target := range []struct {
		name string
		port uint16
	}{
		{"codeup.aliyun.com", 22}, {"gitee.com", 22}, {"database.example.com", 3306},
	} {
		t.Run(target.name, func(t *testing.T) {
			ip, count := fakeIPDNSQuery(t, address, target.name, 1)
			if count != 1 || !netip.MustParsePrefix("198.18.0.0/15").Contains(ip) {
				t.Fatalf("A answer = %s (%d records)", ip, count)
			}
			if firstIP.IsValid() && firstIP == ip {
				t.Fatal("distinct domain reused an existing FakeIP")
			}
			if !firstIP.IsValid() {
				firstIP = ip
			}
			_, count = fakeIPDNSQuery(t, address, target.name, 28)
			if count != 0 {
				t.Fatalf("AAAA returned %d answers in IPv4 mode", count)
			}
			fakeIPCheckConnection(t, address, ip, target.port, requests, target.name, 3)
		})
	}
	t.Run("literal IP remains IP", func(t *testing.T) {
		fakeIPCheckConnection(t, address, netip.MustParseAddr("203.0.113.9"), 22, requests, "203.0.113.9", 1)
	})
	t.Run("TXT uses remote DNS through proxy", func(t *testing.T) {
		_, count := fakeIPDNSQuery(t, address, "example.com", 16)
		if count != 1 {
			t.Fatalf("TXT records = %d; want 1", count)
		}
		select {
		case request := <-requests:
			if request.err != nil || request.host != "203.0.113.53" || request.port != 53 {
				t.Fatalf("remote DNS request %+v", request)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("TXT did not reach remote DNS")
		}
	})
	stop()
	stop = startFakeIPBox(t, binary, path, address)
	t.Run("mapping survives restart without new DNS query", func(t *testing.T) { fakeIPCheckConnection(t, address, firstIP, 22, requests, "codeup.aliyun.com", 3) })
	t.Run("missing mapping rejected", func(t *testing.T) {
		conn, err := fakeIPSocksConnect(address, netip.MustParseAddr("198.19.255.254"), 22)
		if err == nil {
			defer conn.Close()
			b := make([]byte, 1)
			if _, err = conn.Read(b); err == nil {
				t.Fatal("unmapped FakeIP accepted")
			}
		}
		select {
		case request := <-requests:
			t.Fatalf("unmapped address reached upstream: %+v", request)
		default:
		}
	})
	stop()
}

func TestFakeIPHTTPConnect(t *testing.T) {
	binary := fakeIPBinary(t)
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	type connectResult struct {
		request *http.Request
		err     error
	}
	requests := make(chan connectResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := upstream.Accept()
		if err != nil {
			requests <- connectResult{err: err}
			return
		}
		defer conn.Close()
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			requests <- connectResult{err: err}
			return
		}
		request, err := http.ReadRequest(bufio.NewReader(conn))
		if err == nil {
			defer request.Body.Close()
			// Send the target greeting without waiting for application bytes.
			_, err = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\nserver greeting\n")
		}
		requests <- connectResult{request: request, err: err}
	}()
	t.Cleanup(func() { upstream.Close(); <-done })

	cfg := config.Default()
	cfg.ProxyProtocol = config.ProxyProtocolHTTP
	cfg.HTTPPort = upstream.Addr().(*net.TCPAddr).Port
	data, err := SingBoxJSON(cfg, netip.MustParseAddr("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	path, address := fakeIPLoopbackConfig(t, doc)
	startFakeIPBox(t, binary, path, address)
	ip, count := fakeIPDNSQuery(t, address, "codeup.aliyun.com", 1)
	if count != 1 || !netip.MustParsePrefix("198.18.0.0/15").Contains(ip) {
		t.Fatalf("A answer = %s (%d records)", ip, count)
	}
	conn, err := fakeIPSocksConnect(address, ip, 22)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	greeting := make([]byte, len("server greeting\n"))
	if _, err := io.ReadFull(conn, greeting); err != nil {
		t.Fatalf("read server-first greeting: %v", err)
	}
	if string(greeting) != "server greeting\n" {
		t.Fatalf("greeting = %q", greeting)
	}
	select {
	case result := <-requests:
		if result.err != nil {
			t.Fatal(result.err)
		}
		request := result.request
		if request.Method != http.MethodConnect || request.Host != "codeup.aliyun.com:22" || request.RequestURI != "codeup.aliyun.com:22" {
			t.Fatalf("upstream request = %s %s host=%s", request.Method, request.RequestURI, request.Host)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP CONNECT did not reach upstream")
	}
}

func fakeIPBinary(t *testing.T) string {
	t.Helper()
	binary := os.Getenv("WSL_NETGUARD_SING_BOX")
	if binary == "" {
		t.Skip("set WSL_NETGUARD_SING_BOX to sing-box v1.13.13")
	}
	version, err := exec.Command(binary, "version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(version), "sing-box version 1.13.13\n") {
		t.Fatalf("expected sing-box 1.13.13, got %s", version)
	}
	return binary
}

// fakeIPLoopbackConfig changes only delivery and local persistence. Generated
// DNS and routing rules remain intact for the real sing-box protocol checks.
func fakeIPLoopbackConfig(t *testing.T, doc map[string]any) (string, string) {
	t.Helper()
	dir := t.TempDir()
	experimental := doc["experimental"].(map[string]any)
	experimental["cache_file"].(map[string]any)["path"] = filepath.Join(dir, "cache.db")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	doc["inbounds"] = []any{map[string]any{"type": "mixed", "tag": "tun-in", "listen": "127.0.0.1", "listen_port": port}}
	// Interface binding is a TUN loop-avoidance concern, not part of this test.
	doc["route"].(map[string]any)["auto_detect_interface"] = false
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sing-box.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path, address
}

type fakeIPRequest struct {
	host string
	port uint16
	atyp byte
	err  error
}

func fakeIPMockSOCKS(t *testing.T) (net.Listener, <-chan fakeIPRequest) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan fakeIPRequest, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			request := serveFakeIPSOCKS(conn)
			requests <- request
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	return listener, requests
}

func serveFakeIPSOCKS(conn net.Conn) (result fakeIPRequest) {
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		result.err = err
		return
	}
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		result.err = err
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		result.err = err
		return
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		result.err = err
		return
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil {
		result.err = err
		return
	}
	if request[0] != 5 || request[1] != 1 {
		result.err = fmt.Errorf("unexpected SOCKS request %v", request)
		return
	}
	result.atyp = request[3]
	var size int
	switch result.atyp {
	case 1:
		size = 4
	case 4:
		size = 16
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			result.err = err
			return
		}
		size = int(length[0])
	default:
		result.err = fmt.Errorf("invalid ATYP %d", result.atyp)
		return
	}
	destination := make([]byte, size+2)
	if _, err := io.ReadFull(conn, destination); err != nil {
		result.err = err
		return
	}
	result.port = binary.BigEndian.Uint16(destination[size:])
	if result.atyp == 3 {
		result.host = string(destination[:size])
	} else {
		result.host = net.IP(destination[:size]).String()
	}
	if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
		result.err = err
		return
	}
	if result.port == 53 {
		length := make([]byte, 2)
		if _, err := io.ReadFull(conn, length); err != nil {
			result.err = err
			return
		}
		query := make([]byte, int(binary.BigEndian.Uint16(length)))
		if _, err := io.ReadFull(conn, query); err != nil {
			result.err = err
			return
		}
		if len(query) < 12 {
			result.err = fmt.Errorf("truncated DNS query")
			return
		}
		query[2] = 0x81
		query[3] = 0x80
		query[7] = 1
		// TXT "ok", compressed answer owner, IN class and 60 second TTL.
		query = append(query, 0xc0, 0x0c, 0, 16, 0, 1, 0, 0, 0, 60, 0, 3, 2, 'o', 'k')
		frame := binary.BigEndian.AppendUint16(nil, uint16(len(query)))
		_, result.err = conn.Write(append(frame, query...))
		return
	}
	// Server-first traffic ensures SSH/MySQL do not rely on client payload sniffing.
	_, result.err = conn.Write([]byte("server greeting\n"))
	return
}

func startFakeIPBox(t *testing.T, binary, path, address string) func() {
	t.Helper()
	logfile, err := os.CreateTemp(t.TempDir(), "sing-box-log")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "-c", path)
	cmd.Stdout = logfile
	cmd.Stderr = logfile
	if err := cmd.Start(); err != nil {
		logfile.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("sing-box exited: %v", err)
			}
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Error("sing-box failed to stop gracefully")
		}
		logfile.Close()
		if t.Failed() {
			logs, _ := os.ReadFile(logfile.Name())
			t.Logf("sing-box logs:\n%s", logs)
		}
	}
	t.Cleanup(stop)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return stop
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("sing-box listener did not become ready")
	return stop
}

func fakeIPSocksConnect(address string, destination netip.Addr, port uint16) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			conn.Close()
		}
	}()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return nil, err
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return nil, err
	}
	if greeting[0] != 5 || greeting[1] != 0 {
		return nil, fmt.Errorf("invalid SOCKS greeting %v", greeting)
	}
	request := []byte{5, 1, 0, 1}
	request = append(request, destination.AsSlice()...)
	request = binary.BigEndian.AppendUint16(request, port)
	if _, err := conn.Write(request); err != nil {
		return nil, err
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return nil, err
	}
	if reply[1] != 0 {
		return nil, fmt.Errorf("SOCKS rejected connection: %d", reply[1])
	}
	size := 4
	switch reply[3] {
	case 1:
	case 4:
		size = 16
	case 3:
		b := make([]byte, 1)
		if _, err := io.ReadFull(conn, b); err != nil {
			return nil, err
		}
		size = int(b[0])
	default:
		return nil, fmt.Errorf("invalid reply ATYP %d", reply[3])
	}
	if _, err := io.CopyN(io.Discard, conn, int64(size+2)); err != nil {
		return nil, err
	}
	failed = false
	return conn, nil
}

func fakeIPCheckConnection(t *testing.T, address string, ip netip.Addr, port uint16, requests <-chan fakeIPRequest, wantHost string, wantType byte) {
	t.Helper()
	conn, err := fakeIPSocksConnect(address, ip, port)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	greeting := make([]byte, len("server greeting\n"))
	if _, err := io.ReadFull(conn, greeting); err != nil {
		t.Fatal(err)
	}
	if string(greeting) != "server greeting\n" {
		t.Fatalf("greeting %q", greeting)
	}
	select {
	case request := <-requests:
		if request.err != nil || request.host != wantHost || request.port != port || request.atyp != wantType {
			t.Fatalf("upstream request %+v; want %s:%d ATYP %d", request, wantHost, port, wantType)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no upstream SOCKS request")
	}
}

func fakeIPDNSQuery(t *testing.T, address, name string, qtype uint16) (netip.Addr, int) {
	t.Helper()
	conn, err := fakeIPSocksConnect(address, netip.MustParseAddr("1.1.1.1"), 53)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	query := []byte{0, 1, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	for _, label := range strings.Split(name, ".") {
		query = append(query, byte(len(label)))
		query = append(query, label...)
	}
	query = append(query, 0)
	query = binary.BigEndian.AppendUint16(query, qtype)
	query = append(query, 0, 1)
	frame := binary.BigEndian.AppendUint16(nil, uint16(len(query)))
	frame = append(frame, query...)
	if _, err := conn.Write(frame); err != nil {
		t.Fatal(err)
	}
	length := make([]byte, 2)
	if _, err := io.ReadFull(conn, length); err != nil {
		t.Fatal(err)
	}
	answer := make([]byte, int(binary.BigEndian.Uint16(length)))
	if _, err := io.ReadFull(conn, answer); err != nil {
		t.Fatal(err)
	}
	if len(answer) < 12 || answer[3]&15 != 0 {
		t.Fatalf("DNS error response %x", answer)
	}
	count := int(binary.BigEndian.Uint16(answer[6:8]))
	if count == 0 {
		return netip.Addr{}, 0
	}
	// Skip question, then the answer's possibly compressed name.
	offset := 12
	skipName := func() {
		for offset < len(answer) {
			n := int(answer[offset])
			offset++
			if n == 0 {
				return
			}
			if n&192 == 192 {
				offset++
				return
			}
			offset += n
		}
	}
	skipName()
	offset += 4
	skipName()
	if offset+10 > len(answer) {
		t.Fatalf("truncated DNS answer %x", answer)
	}
	recordType := binary.BigEndian.Uint16(answer[offset:])
	size := int(binary.BigEndian.Uint16(answer[offset+8:]))
	offset += 10
	if qtype == 16 && recordType == 16 && offset+size <= len(answer) && string(answer[offset:offset+size]) == "\x02ok" {
		return netip.Addr{}, count
	}
	if recordType != 1 || size != 4 || offset+size > len(answer) {
		t.Fatalf("unexpected address answer %x", answer)
	}
	ip, ok := netip.AddrFromSlice(answer[offset : offset+size])
	if !ok {
		t.Fatal("invalid DNS address")
	}
	return ip, count
}
