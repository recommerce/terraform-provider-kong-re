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
	moduleRoot, _ := filepath.Abs(".")
	if filepath.Base(moduleRoot) == "kong" {
		moduleRoot = filepath.Dir(moduleRoot)
	}

	binaryPath := filepath.Join(moduleRoot, "terraform-provider-kong")
	log.Printf("Building provider at: %s", binaryPath)
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = moduleRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("Build failed: %v\n%s", err, string(output))
	}

	log.Printf("Provider binary built successfully")

	// Verify binary works
	fileInfo, err := os.Stat(binaryPath)
	if err != nil {
		log.Fatalf("Provider binary not found: %v", err)
	}
	log.Printf("DEBUG: Binary size: %d bytes, permissions: %o", fileInfo.Size(), fileInfo.Mode())

	// Set up .terraformrc
	homeDir, _ := os.UserHomeDir()
	terraformrcPath := filepath.Join(homeDir, ".terraformrc")

	// Use simple format without dev_overrides - just point directly
	terraformrc := fmt.Sprintf(`provider_installation {
  dev_overrides {
    "kong" = "%s"
  }
  direct {
    exclude = ["kong"]
  }
}
`, binaryPath)

	if err := os.WriteFile(terraformrcPath, []byte(terraformrc), 0600); err != nil {
		log.Fatalf("Failed to write .terraformrc: %v", err)
	}

	log.Printf("Terraform config written to: %s", terraformrcPath)
	content, _ := os.ReadFile(terraformrcPath)
	log.Printf("DEBUG: .terraformrc content:\n%s", string(content))

	// Rest of TestMain...
	testContext := containers.StartKong(
		defaultKongRepository,
		GetEnvVarOrDefault("KONG_VERSION", defaultKongVersion),
		defaultKongLicense,
	)

	if err := waitForKongReady(testContext.KongHostAddress); err != nil {
		log.Fatalf("Kong failed to become ready: %v", err)
	}

	err = os.Setenv(EnvKongAdminHostAddress, testContext.KongHostAddress)
	if err != nil {
		log.Fatalf("Could not set kong host address env variable: %v", err)
	}
	err = os.Setenv(EnvKongAdminUsername, "AnUsername")
	if err != nil {
		log.Fatalf("Could not set kong admin username env variable: %v", err)
	}
	err = os.Setenv(EnvKongAdminPassword, "AnyPassword")
	if err != nil {
		log.Fatalf("Could not set kong admin password env variable: %v", err)
	}

	log.Printf("DEBUG: Kong Admin Address set to: %s", testContext.KongHostAddress)

	code := m.Run()

	containers.StopKong(testContext)

	os.Exit(code)
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
