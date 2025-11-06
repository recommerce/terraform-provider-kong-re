package kong

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/kevholditch/terraform-provider-kong/kong/containers"
)

const defaultKongVersion = "2.5.0-ubuntu"
const EnvKongAdminHostAddress = "KONG_ADMIN_ADDR"
const EnvKongAdminUsername = "KONG_ADMIN_USERNAME"
const EnvKongAdminPassword = "KONG_ADMIN_PASSWORD"
const defaultKongRepository = "kong"
const defaultKongLicense = ""
const providerNameKong = "kong"

var (
	testAccProviders         map[string]*schema.Provider
	testAccProvider          *schema.Provider
	testAccProviderFactories map[string]func() (*schema.Provider, error)
)

func init() {
	testAccProvider = Provider()
	testAccProviders = map[string]*schema.Provider{
		providerNameKong: testAccProvider,
	}
	testAccProviderFactories = map[string]func() (*schema.Provider, error){
		providerNameKong: func() (*schema.Provider, error) { return Provider(), nil }, //nolint:unparam
	}
}

func TestProvider(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ = Provider()
}

func TestProvider_configure(t *testing.T) {

	rc := terraform.NewResourceConfigRaw(map[string]interface{}{})
	p := Provider()
	err := p.Configure(context.Background(), rc)
	if err != nil {
		t.Fatal(err)
	}
}

func TestProvider_configure_strict(t *testing.T) {

	rc := terraform.NewResourceConfigRaw(map[string]interface{}{
		"strict_plugins_match": "true",
	})
	p := Provider()
	err := p.Configure(context.Background(), rc)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMain(m *testing.M) {
	// Build the provider binary
	if err := buildProviderBinary(); err != nil {
		log.Fatalf("Failed to build provider binary: %v", err)
	}

	// Setup Terraform dev overrides
	if err := setupTerraformOverrides(); err != nil {
		log.Fatalf("Failed to setup Terraform overrides: %v", err)
	}

	testContext := containers.StartKong(defaultKongRepository, GetEnvVarOrDefault("KONG_VERSION", defaultKongVersion), defaultKongLicense)

	// Wait for Kong to be ready
	if err := waitForKongReady(testContext.KongHostAddress); err != nil {
		log.Fatalf("Kong failed to become ready: %v", err)
	}

	err := os.Setenv(EnvKongAdminHostAddress, testContext.KongHostAddress)
	if err != nil {
		log.Fatalf("Could not set kong host address env variable: %v", err)
	}

	// DEBUG: Print what Kong URL we're using
	log.Printf("DEBUG: Kong Admin Address set to: %s", testContext.KongHostAddress)

	err = os.Setenv(EnvKongAdminPassword, "AnUsername")
	if err != nil {
		log.Fatalf("Could not set kong admin username env variable: %v", err)
	}
	err = os.Setenv(EnvKongAdminPassword, "AnyPassword")
	if err != nil {
		log.Fatalf("Could not set kong admin password env variable: %v", err)
	}

	code := m.Run()

	containers.StopKong(testContext)

	os.Exit(code)

}

func buildProviderBinary() error {
	// Find the repo root (go up from ./kong to parent)
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// If we're in ./kong, go up one level to find go.mod
	moduleRoot := currentDir
	if filepath.Base(currentDir) == "kong" {
		moduleRoot = filepath.Dir(currentDir)
	}

	binaryPath := filepath.Join(moduleRoot, "terraform-provider-kong")

	log.Printf("DEBUG: Current dir: %s", currentDir)
	log.Printf("DEBUG: Repo root: %s", moduleRoot)
	log.Printf("DEBUG: Building provider binary at %s", binaryPath)

	// Build from repo root, using ./main.go
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = moduleRoot

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build provider: %w\nOutput: %s", err, string(output))
	}

	log.Printf("DEBUG: Provider binary built successfully")
	return nil
}

func setupTerraformOverrides() error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// If we're in ./kong, go up one level
	moduleRoot := currentDir
	if filepath.Base(currentDir) == "kong" {
		moduleRoot = filepath.Dir(currentDir)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	terraformrcPath := filepath.Join(homeDir, ".terraformrc")

	terraformrc := fmt.Sprintf(`provider_installation {
  dev_overrides {
    "kevholditch/kong" = "%s"
  }
  direct {}
}
`, moduleRoot)

	if err := os.WriteFile(terraformrcPath, []byte(terraformrc), 0600); err != nil {
		return fmt.Errorf("failed to write .terraformrc: %w", err)
	}

	log.Printf("DEBUG: Terraform overrides written to %s", terraformrcPath)
	log.Printf("DEBUG: Provider path override: %s", moduleRoot)
	return nil
}

// Add this helper function
func waitForKongReady(kongURL string) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	log.Printf("DEBUG: Waiting for Kong at %s", kongURL)
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		resp, err := client.Get(kongURL + "/status")
		if err != nil {
			log.Printf("DEBUG: Attempt %d - Error: %v", i+1, err)
			time.Sleep(1 * time.Second)
			continue
		}

		if resp.StatusCode == 200 {
			resp.Body.Close()
			log.Printf("DEBUG: Kong is ready at %s", kongURL)
			return nil
		}

		log.Printf("DEBUG: Attempt %d - Status: %d", i+1, resp.StatusCode)
		resp.Body.Close()
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("Kong not ready after %d seconds at %s", maxRetries, kongURL)
}
