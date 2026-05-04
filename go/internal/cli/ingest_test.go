package cli

import "testing"

// 본 패키지의 fredDefaultBaseURL이 정확한 FRED API endpoint인지 검증.
// 잘못된 URL이면 production API 호출 자체가 실패하므로 sanity check.
func TestFREDDefaultBaseURL(t *testing.T) {
	if fredDefaultBaseURL != "https://api.stlouisfed.org/fred" {
		t.Errorf("fredDefaultBaseURL 기대 https://api.stlouisfed.org/fred, 실제 %q", fredDefaultBaseURL)
	}
}
