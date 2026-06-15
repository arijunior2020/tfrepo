package cli

import (
	"strings"
	"testing"
)

func TestRenderBannerIncludesSubtitleAndArt(t *testing.T) {
	got := renderBanner()

	if !strings.Contains(got, bannerSubtitle) {
		t.Errorf("renderBanner() missing subtitle: %q", got)
	}
	if strings.Count(got, "\n") < 6 {
		t.Errorf("renderBanner() has too few lines: %q", got)
	}
}
