package version

import "testing"

func TestVersionDomains(t *testing.T) {
	for _, value := range []string{"4.7", "4.5.2", "4.8-dev3", "4.7.2-rc1", "4.7.1-beta2"} {
		if !ValidEngine(value) {
			t.Fatalf("supported engine version %q was rejected", value)
		}
	}
	for _, value := range []string{"8.0.410", "11.0.100-preview.7.26381.103", "8.0.100-rc.2.23502.12"} {
		if !ValidSDK(value) {
			t.Fatalf("supported SDK version %q was rejected", value)
		}
	}
	for _, value := range []string{"latest", "4.8-stable", "11.0.100-preview.7.26381.103"} {
		if ValidEngine(value) {
			t.Fatalf("invalid engine version %q was accepted", value)
		}
	}
	for _, value := range []string{"4.8-dev3", "8.0", "8.0.410-beta1"} {
		if ValidSDK(value) {
			t.Fatalf("invalid SDK version %q was accepted", value)
		}
	}
}

func TestValidEngineID(t *testing.T) {
	for _, value := range []string{"4.7-standard", "4.8-dev3-dotnet", "4.7.2-rc1-standard"} {
		if !ValidEngineID(value) {
			t.Fatalf("supported engine ID %q was rejected", value)
		}
	}
	for _, value := range []string{"4.7", "4.7-mono", "11.0.100-preview.7.26381.103-standard"} {
		if ValidEngineID(value) {
			t.Fatalf("invalid engine ID %q was accepted", value)
		}
	}
}
