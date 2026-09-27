package endpointguard

import "testing"

func TestValidateAllowsNormalEndpoints(t *testing.T) {
	for _, raw := range []string{
		"",
		"https://api.openai.com/v1",
		"http://127.0.0.1:11434/v1",
		"http://192.168.1.20:8188",
		"http://localhost:7860",
	} {
		if err := Validate(raw); err != nil {
			t.Errorf("Validate(%q) 不应报错: %v", raw, err)
		}
	}
}

func TestValidateBlocksMetadataAndLinkLocal(t *testing.T) {
	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://[fe80::1]/",
		"http://100.100.100.200/latest/meta-data/",
		"http://[fd00:ec2::254]/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://metadata/computeMetadata/v1/",
	} {
		if err := Validate(raw); err == nil {
			t.Errorf("Validate(%q) 应被拒绝", raw)
		}
	}
}

func TestValidateRejectsMalformed(t *testing.T) {
	for _, raw := range []string{"ftp://example.com", "not a url", "http://"} {
		if err := Validate(raw); err == nil {
			t.Errorf("Validate(%q) 应报错", raw)
		}
	}
}
