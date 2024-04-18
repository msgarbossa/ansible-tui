package main

import (
	"log/slog"
	"net"
	"os"
	"testing"
	"time"
)

// global
var (
	pb          PlaybookConfig
	projectRoot string
	httpPort    string
)

const (
	httpIp = "127.0.0.1"
)

func tcpGather(ip string, ports []string) map[string]string {
	// check emqx 1883, 8083 port

	results := make(map[string]string)
	for _, port := range ports {
		address := net.JoinHostPort(ip, port)
		// 3 second timeout
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err != nil {
			results[port] = "failed"
			// todo log handler
		} else {
			if conn != nil {
				results[port] = "success"
				_ = conn.Close()
			} else {
				results[port] = "failed"
			}
		}
	}
	return results
}

func TestValidInputs(t *testing.T) {

	slog.SetLogLoggerLevel(slog.LevelDebug)

	// unset Python virtualenv if it is set
	if os.Getenv("VIRTUAL_ENV") != "" {
		slog.Info("unsetting VIRTUAL_ENV")
		os.Unsetenv("VIRTUAL_ENV")
	}

	pb = PlaybookConfig{}
	os.Chdir("../../")
	projectRoot, _ = os.Getwd()
	os.Setenv("PB_CONFIG_FILE", "./test/as-venv-otel.yml")
	os.Setenv("INVENTORY_FILE", "./test/inventory-localhost.txt")
	os.Setenv("PLAYBOOK", "./test/playbook-simple.yml")

	err := pb.readConf(os.Getenv("PB_CONFIG_FILE"))
	if err != nil {
		t.Errorf("Expected no errors reading config file, got %s", err)
	}

	err = pb.readEnvs()
	if err != nil {
		t.Errorf("Expected no errors reading environment variables, got %s", err)
	}

	err = pb.validateInputs()
	if err != nil {
		t.Errorf("Expected no errors validating inputs, got %s", err)
	}
}

func TestInvalidInputs(t *testing.T) {
	os.Setenv("INVENTORY_FILE", "./test/inventory-localhost2.txt")

	err := pb.readEnvs()
	if err != nil {
		t.Errorf("Expected no errors reading environment variables, got %s", err)
	}

	err = pb.validateInputs()
	if err == nil {
		t.Errorf("Expected errors validating inputs, got %s", err)
	}
}

func TestInvalidInventory(t *testing.T) {
	os.Setenv("INVENTORY_FILE", "./test/inventory-invalid.txt")

	err := pb.readEnvs()
	if err != nil {
		t.Errorf("Expected no errors reading environment variables, got %s", err)
	}

	err = pb.validateInputs()
	if err != nil {
		t.Errorf("Expected no errors validating inputs, got %s", err)
	}

	err = pb.processInputs()
	if err != nil {
		t.Errorf("Expected no errors processing inputs, got %s", err)
	}

	// inventory validation is done just before playbook execution
	rc, err := pb.runAnsiblePlaybook()
	if err == nil {
		t.Errorf("Expected errors validating inventory, got %s", err)
	}
	if rc == 0 {
		t.Errorf("Expected non-zero return code running playbook, got %d", rc)
	}

}

func TestSimplePlaybookVenv(t *testing.T) {

	os.Setenv("INVENTORY_FILE", "./test/inventory-localhost.txt")

	err := pb.readEnvs()
	if err != nil {
		t.Errorf("Expected no errors reading environment variables, got %s", err)
	}

	err = pb.validateInputs()
	if err != nil {
		t.Errorf("Expected no errors validating inputs, got %s", err)
	}

	err = pb.processInputs()
	if err != nil {
		t.Errorf("Expected no errors processing inputs, got %s", err)
	}

	rc, err := pb.runAnsiblePlaybook()
	if err != nil {
		t.Errorf("Expected no errors running playbook, got %s", err)
	}
	if rc != 0 {
		t.Errorf("Expected exit code 0 running playbook, got %d", rc)
	}

}

func TestSimplePlaybookContainer(t *testing.T) {

	os.Setenv("PB_CONFIG_FILE", "./test/as-container.yml")

	err := pb.readConf(os.Getenv("PB_CONFIG_FILE"))
	if err != nil {
		t.Errorf("Expected no errors reading config file, got %s", err)
	}

	err = pb.readEnvs()
	if err != nil {
		t.Errorf("Expected no errors reading environment variables, got %s", err)
	}

	err = pb.validateInputs()
	if err != nil {
		t.Errorf("Expected no errors validating inputs, got %s", err)
	}

	err = pb.processInputs()
	if err != nil {
		t.Errorf("Expected no errors processing inputs, got %s", err)
	}

	rc, err := pb.runAnsiblePlaybook()
	if err != nil {
		t.Errorf("Expected no errors running playbook, got %s", err)
	}
	if rc != 0 {
		t.Errorf("Expected exit code 0 running playbook, got %d", rc)
	}

}

// func TestHttpListener(t *testing.T) {

// 	// var (
// 	// 	ctx, cancel = context.WithCancel(context.Background())
// 	// )

// 	httpAddr = ":6000"
// 	httpPort = "6000"

// 	// defer func() {
// 	// 	fmt.Println("Running cancel()")
// 	// 	cancel()
// 	// }()

// 	if !regExpHostPort.MatchString(httpAddr) {
// 		slog.Error("httpAddr does not match expected hostname:port pattern")
// 		return
// 	}

// 	// start listener
// 	//go startHttpListener(ctx)
// 	go mainListener()
// 	time.Sleep(time.Second * 1)

// 	httpPorts := []string{httpPort}

// 	results := tcpGather(httpIp, httpPorts)
// 	for port, status := range results {
// 		slog.Info(fmt.Sprintf("port test: %s status: %s\n", port, status))
// 	}
// 	if results[httpPort] != "success" {
// 		t.Errorf("expected port %s == success but got %s", httpPort, results[httpPort])
// 	}

// }

// func TestDataIngestion(t *testing.T) {

// 	// slurp entire file contents into memory
// 	contents, err := os.ReadFile("./test/ansible-shim.json")
// 	if err != nil {
// 		t.Error("failed to read JSON input")
// 	}

// 	requestURL := fmt.Sprintf("http://localhost:%s/ansible", httpPort)

// 	// convert contents to bytes
// 	var b bytes.Buffer
// 	gz := gzip.NewWriter(&b)
// 	if _, err := gz.Write([]byte(contents)); err != nil {
// 		t.Errorf("Expected webhook test body to be valid: %s", err)
// 	}
// 	if err := gz.Close(); err != nil {
// 		t.Fatal((err))
// 	}

// 	req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewBuffer(b.Bytes()))
// 	if err != nil {
// 		fmt.Printf("client: could not create post test request: %s\n", err)
// 		os.Exit(1)
// 	}
// 	req.Header.Set("Content-Type", "application/json")
// 	req.Header.Set("Content-Encoding", "gzip")

// 	client := http.Client{
// 		Timeout: 10 * time.Second,
// 	}

// 	for i := 0; i < 10; i++ {
// 		resp, err := client.Do(req)
// 		if err != nil {
// 			t.Errorf("Expected HTTP POST for webhook test to be successful: %s", err)
// 		}
// 		t.Log(resp)

// 		// need to close response body even if you don't want to read it.
// 		defer resp.Body.Close()
// 		if resp.StatusCode != http.StatusOK {
// 			log.Println("Non-OK HTTP status:", resp.StatusCode)
// 		}
// 	}

// 	// TODO: verify results/parsing

// 	// clear the buffer before additional requests (or else the previous entry still exists)
// 	// b.Reset()

// }
