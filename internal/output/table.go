package output

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	v2_metric "github.com/goark/go-cvss/v2/metric"
	v3_metric "github.com/goark/go-cvss/v3/metric"
	"golang.org/x/exp/maps"

	"github.com/google/osv-scanner/pkg/models"
	"github.com/google/osv-scanner/pkg/osv"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

// PrintTableResults prints the osv scan results into a human friendly table.
func PrintTableResults(vulnResult *models.VulnerabilityResults, outputWriter io.Writer, terminalWidth int) {
	// Render the vulnerabilities.
	outputTable := newTable(outputWriter, terminalWidth)
	outputTable = tableBuilder(outputTable, vulnResult, terminalWidth > 0)
	if outputTable.Length() != 0 {
		outputTable.Render()
	}

	// Render the licenses if any.
	outputLicenseTable := newTable(outputWriter, terminalWidth)
	outputLicenseTable = licenseTableBuilder(outputLicenseTable, vulnResult)
	if outputLicenseTable.Length() == 0 {
		return
	}
	outputLicenseTable.Render()
}

func newTable(outputWriter io.Writer, terminalWidth int) table.Writer {
	outputTable := table.NewWriter()
	outputTable.SetOutputMirror(outputWriter)

	if terminalWidth > 0 { // If output is a terminal, set max length to width and add styling
		outputTable.SetStyle(table.StyleRounded)
		outputTable.Style().Color.Row = text.Colors{text.Reset, text.BgHiBlack}
		outputTable.Style().Color.RowAlternate = text.Colors{text.Reset, text.BgBlack}
		outputTable.Style().Options.DoNotColorBordersAndSeparators = true
		outputTable.SetAllowedRowLength(terminalWidth)
	} // Otherwise use default ascii (e.g. getting piped to a file)
	return outputTable

}

func tableBuilder(outputTable table.Writer, vulnResult *models.VulnerabilityResults, addStyling bool) table.Writer {
	outputTable.AppendHeader(table.Row{"OSV URL", "CVSS", "Ecosystem", "Package", "Version", "Source"})
	rows := tableBuilderInner(vulnResult, addStyling, true)
	for _, elem := range rows {
		outputTable.AppendRow(elem.row, table.RowConfig{AutoMerge: elem.shouldMerge})
	}

	uncalledRows := tableBuilderInner(vulnResult, addStyling, false)
	if len(uncalledRows) == 0 {
		return outputTable
	}

	outputTable.AppendSeparator()
	outputTable.AppendRow(table.Row{"Uncalled vulnerabilities"})
	outputTable.AppendSeparator()

	for _, elem := range uncalledRows {
		outputTable.AppendRow(elem.row, table.RowConfig{AutoMerge: elem.shouldMerge})
	}

	return outputTable
}

type tbInnerResponse struct {
	row         table.Row
	shouldMerge bool
}

func tableBuilderInner(vulnResult *models.VulnerabilityResults, addStyling bool, calledVulns bool) []tbInnerResponse {
	allOutputRows := []tbInnerResponse{}
	// Working directory used to simplify path
	workingDir, err := os.Getwd()
	if err != nil {
		log.Panicf("can't get working dir: %v", err)
	}
	for _, sourceRes := range vulnResult.Results {
		for _, pkg := range sourceRes.Packages {
			source := sourceRes.Source
			sourcePath, err := filepath.Rel(workingDir, source.Path)
			if err == nil { // Simplify the path if possible
				source.Path = sourcePath
			}

			// Merge groups into the same row
			for _, group := range pkg.Groups {
				if group.IsCalled() != calledVulns {
					continue
				}

				outputRow := table.Row{}
				shouldMerge := false

				var links []string

				for _, vuln := range group.IDs {
					if addStyling {
						links = append(links, osv.BaseVulnerabilityURL+text.Bold.EscapeSeq()+vuln+text.Reset.EscapeSeq())
					} else {
						links = append(links, osv.BaseVulnerabilityURL+vuln)
					}
				}

				outputRow = append(outputRow, strings.Join(links, "\n"))
				outputRow = append(outputRow, MaxSeverity(group, pkg))

				if pkg.Package.Ecosystem == "GIT" {
					outputRow = append(outputRow, "GIT", pkg.Package.Version, pkg.Package.Version)
					shouldMerge = true
				} else {
					outputRow = append(outputRow, pkg.Package.Ecosystem, pkg.Package.Name, pkg.Package.Version)
				}

				outputRow = append(outputRow, source.Path)
				allOutputRows = append(allOutputRows, tbInnerResponse{
					row:         outputRow,
					shouldMerge: shouldMerge,
				})
			}
		}
	}

	return allOutputRows
}

func MaxSeverity(group models.GroupInfo, pkg models.PackageVulns) string {
	var maxSeverity float64
	for _, vulnID := range group.IDs {
		var severities []models.Severity
		for _, vuln := range pkg.Vulnerabilities {
			if vuln.ID == vulnID {
				severities = vuln.Severity
			}
		}
		for _, severity := range severities {
			switch severity.Type {
			case models.SeverityCVSSV2:
				numericSeverity, _ := v2_metric.NewBase().Decode(severity.Score)
				maxSeverity = math.Max(maxSeverity, numericSeverity.Score())
			case models.SeverityCVSSV3:
				numericSeverity, _ := v3_metric.NewBase().Decode(severity.Score)
				maxSeverity = math.Max(maxSeverity, numericSeverity.Score())
			}
		}
	}

	if maxSeverity == 0 {
		return ""
	}

	return fmt.Sprintf("%v", maxSeverity)
}

func licenseTableBuilder(outputTable table.Writer, vulnResult *models.VulnerabilityResults) table.Writer {
	licenseConfig := vulnResult.ExperimentalAnalysisConfig.Licenses
	if !licenseConfig.Enabled {
		return outputTable
	}
	if len(licenseConfig.Allowlist) == 0 {
		return licenseSummaryTableBuilder(outputTable, vulnResult)
	}

	return licenseViolationsTableBuilder(outputTable, vulnResult)
}

func licenseSummaryTableBuilder(outputTable table.Writer, vulnResult *models.VulnerabilityResults) table.Writer {
	counts := make(map[models.License]int)
	for _, pkgSource := range vulnResult.Results {
		for _, pkg := range pkgSource.Packages {
			for _, l := range pkg.Licenses {
				counts[l] += 1
			}
		}
	}
	if len(counts) == 0 {
		// No packages found.
		return outputTable
	}
	licenses := maps.Keys(counts)
	// Sort the license count in descending count order with the UNKNOWN
	// license last.
	sort.Slice(licenses, func(i, j int) bool {
		if licenses[i] == "UNKNOWN" {
			return false
		}
		if licenses[j] == "UNKNOWN" {
			return true
		}
		if counts[licenses[i]] == counts[licenses[j]] {
			return licenses[i] < licenses[j]
		}
		return counts[licenses[i]] > counts[licenses[j]]
	})
	outputTable.AppendHeader(table.Row{"License", "No. of package versions"})
	for _, license := range licenses {
		outputTable.AppendRow(table.Row{license, counts[license]})
	}
	return outputTable
}

func licenseViolationsTableBuilder(outputTable table.Writer, vulnResult *models.VulnerabilityResults) table.Writer {
	outputTable.AppendHeader(table.Row{"License Violation", "Ecosystem", "Package", "Version", "Source"})
	for _, pkgSource := range vulnResult.Results {
		for _, pkg := range pkgSource.Packages {
			if len(pkg.LicenseViolations) == 0 {
				continue
			}
			violations := make([]string, len(pkg.LicenseViolations))
			for i, l := range pkg.LicenseViolations {
				violations[i] = string(l)
			}
			outputTable.AppendRow(table.Row{
				strings.Join(violations, ", "),
				pkg.Package.Ecosystem,
				pkg.Package.Name,
				pkg.Package.Version,
				pkgSource.Source.Path,
			})
		}
	}

	return outputTable
}
