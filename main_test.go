package main

import (
	"os"
	"testing"
)

// global
var (
	pb PlaybookConfig
)

func TestValidInputs(t *testing.T) {

	pb = PlaybookConfig{}
	os.Setenv("PB_CONFIG_FILE", "./test/ansible-shim.yml")
	os.Setenv("INVENTORY_FILE", "./test/inventory-localhost.txt")
	os.Setenv("PLAYBOOK", "./test/playbook-simple.yml")

	err := pb.readConf(os.Getenv("PB_CONFIG_FILE"))
	if err != nil {
		t.Errorf("Expected no errors reading config file, got %s", err)
	}

	err = pb.readEvs()
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

	err := pb.readEvs()
	if err != nil {
		t.Errorf("Expected no errors reading environment variables, got %s", err)
	}

	err = pb.validateInputs()
	if err == nil {
		t.Errorf("Expected errors validating inputs, got %s", err)
	}
}

func TestSimplePlaybook(t *testing.T) {
	os.Setenv("INVENTORY_FILE", "./test/inventory-localhost.txt")

	err := pb.readEvs()
	if err != nil {
		t.Errorf("Expected no errors reading environment variables, got %s", err)
	}

	err = pb.validateInputs()
	if err != nil {
		t.Errorf("Expected no errors validating inputs, got %s", err)
	}

	rc, err := pb.runAnsiblePlaybook()
	if err != nil {
		t.Errorf("Expected no errors running playbook, got %s", err)
	}
	if rc != 0 {
		t.Errorf("Expected exit code 0 running playbook, got %d", rc)
	}

}
