package platform

import (
	"context"
	"testing"
)

func TestLowestFixedAndCompare(t *testing.T) {
	if got := lowestFixed("1.10.0, 1.9.2"); got != "1.9.2" {
		t.Fatalf("got %q", got)
	}
	if lowestFixed("") != "" {
		t.Fatal("empty")
	}
	if compareVersions("v0.31.0", "v0.9.0") <= 0 {
		t.Fatal("numeric compare")
	}
}

func TestVulnViewRollup(t *testing.T) {
	v := demoVulnSet().view(nil)
	if !v.Available || !v.SBOMAvailable {
		t.Fatalf("availability %+v", v.Sources)
	}
	var pay *VulnImageRollup
	for i := range v.Images {
		if v.Images[i].Image == "ghcr.io/zyvorai/payments:v2.8.4" {
			pay = &v.Images[i]
		}
	}
	if pay == nil {
		t.Fatalf("payments image missing: %+v", v.Images)
	}
	if pay.Counts.Critical != 1 || pay.Counts.High != 3 || pay.Counts.Medium != 2 || pay.Counts.Low != 1 {
		t.Fatalf("counts %+v", pay.Counts)
	}
	if pay.Fixable != 6 {
		t.Fatalf("fixable %d", pay.Fixable)
	}
	if len(pay.Workloads) != 1 || pay.Workloads[0].Kind != "Deployment" || pay.Workloads[0].Name != "payments-api" {
		t.Fatalf("workloads %+v", pay.Workloads)
	}
	// stdlib has two fixes (1.21.11 and 1.22.7): lowest wins.
	found := false
	for _, f := range pay.Fixes {
		if f.Package == "stdlib" {
			found = true
			if f.FixedIn != "1.21.11" || f.Fixes != 2 {
				t.Fatalf("stdlib fix %+v", f)
			}
		}
	}
	if !found {
		t.Fatal("no stdlib fix")
	}
	if !pay.HasSBOM || pay.Components != 7 || len(pay.Licenses) == 0 {
		t.Fatalf("sbom %+v", pay)
	}
	if v.TopCVEs[0].Severity != "CRITICAL" {
		t.Fatalf("top cve %+v", v.TopCVEs[0])
	}
	if len(v.SBOM) != 3 || v.SBOM[0].Unlicensed != 1 {
		t.Fatalf("sbom list %+v", v.SBOM)
	}
}

func TestVulnViewFilters(t *testing.T) {
	d := &DemoProvider{}
	v, err := d.ListVulnerabilities(context.Background(), "commerce", "high")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Images) != 1 || len(v.TopCVEs) != 2 {
		t.Fatalf("filtered %d images %d cves", len(v.Images), len(v.TopCVEs))
	}
	for _, c := range v.TopCVEs {
		if c.Severity != "HIGH" {
			t.Fatalf("severity leak %+v", c)
		}
	}
	if _, err := d.ListVulnerabilities(context.Background(), "", "bogus"); err == nil {
		t.Fatal("expected invalid severity error")
	}
	v, _ = d.ListVulnerabilities(context.Background(), "", "critical")
	if len(v.Images) != 1 {
		t.Fatalf("critical images %d", len(v.Images))
	}
}

func TestVulnViewNotInstalled(t *testing.T) {
	v := vulnSet{Sources: []SecuritySource{{Name: "Trivy VulnerabilityReports", Message: "CRD not installed"}}}.view(nil)
	if v.Available || v.InstallHint == "" {
		t.Fatalf("%+v", v)
	}
}
