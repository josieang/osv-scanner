// main cannot be accessed directly, so cannot use main_test
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
)

func createTestDir(t *testing.T) (string, func()) {
	t.Helper()

	p, err := os.MkdirTemp("", "osv-scanner-test-*")
	if err != nil {
		t.Fatalf("could not create test directory: %v", err)
	}

	return p, func() {
		_ = os.RemoveAll(p)
	}
}

func dedent(t *testing.T, str string) string {
	t.Helper()

	// 0. replace all tabs with spaces
	str = strings.ReplaceAll(str, "\t", "  ")

	// 1. remove trailing whitespace
	re := regexp.MustCompile(`\r?\n([\t ]*)$`)
	str = re.ReplaceAllString(str, "")

	// 2. if any of the lines are not indented, return as we're already dedent-ed
	re = regexp.MustCompile(`(^|\r?\n)[^\t \n]`)
	if re.MatchString(str) {
		return str
	}

	// 3. find all line breaks to determine the highest common indentation level
	re = regexp.MustCompile(`\n[\t ]+`)
	matches := re.FindAllString(str, -1)

	// 4. remove the common indentation from all strings
	if matches != nil {
		size := len(matches[0]) - 1

		for _, match := range matches {
			if len(match)-1 < size {
				size = len(match) - 1
			}
		}

		re := regexp.MustCompile(`\n[\t ]{` + fmt.Sprint(size) + `}`)
		str = re.ReplaceAllString(str, "\n")
	}

	// 5. Remove leading whitespace.
	re = regexp.MustCompile(`^\r?\n`)
	str = re.ReplaceAllString(str, "")

	return str
}

// checks if two strings are equal, treating any occurrences of `%%` in the
// expected string to mean "any text"
func areEqual(t *testing.T, actual, expect string) bool {
	t.Helper()

	expect = regexp.QuoteMeta(expect)
	expect = strings.ReplaceAll(expect, "%%", ".+")

	re := regexp.MustCompile(`^` + expect + `$`)

	return re.MatchString(actual)
}

type cliTestCase struct {
	name         string
	args         []string
	wantExitCode int
	wantStdout   string
	wantStderr   string
}

func testCli(t *testing.T, tc cliTestCase) {
	t.Helper()

	stdoutBuffer := &bytes.Buffer{}
	stderrBuffer := &bytes.Buffer{}

	ec := run(tc.args, stdoutBuffer, stderrBuffer)
	// ec := run(tc.args, os.Stdout, os.Stderr)

	stdout := stdoutBuffer.String()
	stderr := stderrBuffer.String()

	if ec != tc.wantExitCode {
		t.Errorf("cli exited with code %d, not %d", ec, tc.wantExitCode)
	}

	if !areEqual(t, dedent(t, stdout), dedent(t, tc.wantStdout)) {
		t.Errorf("stdout\n got:\n%s\n\n want:\n%s", dedent(t, stdout), dedent(t, tc.wantStdout))
	}

	if !areEqual(t, dedent(t, stderr), dedent(t, tc.wantStderr)) {
		t.Errorf("stderr\n got:\n%s\n\n want:\n%s", dedent(t, stderr), dedent(t, tc.wantStderr))
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []cliTestCase{
		{
			name:         "",
			args:         []string{""},
			wantExitCode: 128,
			wantStdout:   "",
			wantStderr: `
        No package sources found, --help for usage information.
			`,
		},
		{
			name:         "",
			args:         []string{"", "--version"},
			wantExitCode: 0,
			wantStdout: fmt.Sprintf(`
				osv-scanner version: %s
				commit: n/a
				built at: n/a
			`, version),
			wantStderr: "",
		},
		// one specific supported lockfile
		{
			name:         "",
			args:         []string{"", "./fixtures/locks-many/composer.lock"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-many/composer.lock
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
				No issues found
			`,
			wantStderr: "",
		},
		// one specific supported sbom with vulns
		{
			name:         "",
			args:         []string{"", "--config=./fixtures/osv-scanner-empty-config.toml", "./fixtures/sbom-insecure/"},
			wantExitCode: 1,
			wantStdout: `
				Scanning dir ./fixtures/sbom-insecure/
				Scanned %%/fixtures/sbom-insecure/alpine.cdx.xml as CycloneDX SBOM and found 15 packages
				Scanned %%/fixtures/sbom-insecure/postgres-stretch.cdx.xml as CycloneDX SBOM and found 136 packages
				+-------------------------------------+------+-----------+--------------------------------+------------------------------------+-------------------------------------------------+
				| OSV URL                             | CVSS | ECOSYSTEM | PACKAGE                        | VERSION                            | SOURCE                                          |
				+-------------------------------------+------+-----------+--------------------------------+------------------------------------+-------------------------------------------------+
				| https://osv.dev/CVE-2022-37434      |      | Alpine    | zlib                           | 1.2.10-r2                          | fixtures/sbom-insecure/alpine.cdx.xml           |
				| https://osv.dev/DLA-3022-1          |      | Debian    | dpkg                           | 1.18.25                            | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-v95c-p5hm-xq8f | 6    | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GO-2022-0274        |      |           |                                |                                    |                                                 |
				| https://osv.dev/GHSA-f3fp-gc8g-vw66 | 5.9  | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-g2j6-57v7-gm8c | 6.1  | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-m8cg-xc2p-r3fc | 2.5  | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-vpvm-3wq2-2wvm | 7    | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-p782-xgp4-8hr8 | 5.3  | Go        | golang.org/x/sys               | v0.0.0-20210817142637-7d9622a276b7 | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GO-2022-0493        |      |           |                                |                                    |                                                 |
				| https://osv.dev/DLA-3012-1          |      | Debian    | libxml2                        | 2.9.4+dfsg1-2.2+deb9u6             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/DLA-3008-1          |      | Debian    | openssl                        | 1.1.0l-1~deb9u5                    | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/DLA-3051-1          |      | Debian    | tzdata                         | 2021a-0+deb9u3                     | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				+-------------------------------------+------+-----------+--------------------------------+------------------------------------+-------------------------------------------------+
			`,
			wantStderr: "",
		},
		// one specific unsupported lockfile
		{
			name:         "",
			args:         []string{"", "./fixtures/locks-many/not-a-lockfile.toml"},
			wantExitCode: 128,
			wantStdout: `
				Scanning dir ./fixtures/locks-many/not-a-lockfile.toml
			`,
			wantStderr: `
				No package sources found, --help for usage information.
			`,
		},
		// all supported lockfiles in the directory should be checked
		{
			name:         "Scan locks-many",
			args:         []string{"", "./fixtures/locks-many"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-many
				Scanned %%/fixtures/locks-many/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-many/alpine.cdx.xml as CycloneDX SBOM and found 15 packages
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
				Scanned %%/fixtures/locks-many/package-lock.json file and found 1 package
				Scanned %%/fixtures/locks-many/yarn.lock file and found 1 package
				Loaded filter from: %%/fixtures/locks-many/osv-scanner.toml
				GHSA-whgm-jr23-g3j9 has been filtered out because: Test manifest file
				Filtered 1 vulnerability from output
				No issues found
			`,
			wantStderr: "",
		},
		// all supported lockfiles in the directory should be checked
		{
			name:         "",
			args:         []string{"", "./fixtures/locks-many-with-invalid"},
			wantExitCode: 127,
			wantStdout: `
				Scanning dir ./fixtures/locks-many-with-invalid
				Scanned %%/fixtures/locks-many-with-invalid/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-many-with-invalid/yarn.lock file and found 1 package
			`,
			wantStderr: `
				Attempted to scan lockfile but failed: %%/fixtures/locks-many-with-invalid/composer.lock
			`,
		},
		// only the files in the given directories are checked by default (no recursion)
		{
			name:         "",
			args:         []string{"", "./fixtures/locks-one-with-nested"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-one-with-nested
				Scanned %%/fixtures/locks-one-with-nested/yarn.lock file and found 1 package
				No issues found
			`,
			wantStderr: "",
		},
		// nested directories are checked when `--recursive` is passed
		{
			name:         "",
			args:         []string{"", "--recursive", "./fixtures/locks-one-with-nested"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-one-with-nested
				Scanned %%/fixtures/locks-one-with-nested/nested/composer.lock file and found 1 package
				Scanned %%/fixtures/locks-one-with-nested/yarn.lock file and found 1 package
				No issues found
			`,
			wantStderr: "",
		},
		// .gitignored files
		{
			name:         "",
			args:         []string{"", "--recursive", "./fixtures/locks-gitignore"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-gitignore
				Scanned %%/fixtures/locks-gitignore/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/subdir/yarn.lock file and found 1 package
				No issues found
			`,
			wantStderr: "",
		},
		// ignoring .gitignore
		{
			name:         "",
			args:         []string{"", "--recursive", "--no-ignore", "./fixtures/locks-gitignore"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-gitignore
				Scanned %%/fixtures/locks-gitignore/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/composer.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/ignored/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/ignored/yarn.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/subdir/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/subdir/composer.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/subdir/yarn.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/yarn.lock file and found 1 package
				No issues found
			`,
			wantStderr: "",
		},
		// output with json
		{
			name:         "",
			args:         []string{"", "--json", "./fixtures/locks-many/composer.lock"},
			wantExitCode: 0,
			wantStdout: `
				{
					"results": []
				}
			`,
			wantStderr: `
				Scanning dir ./fixtures/locks-many/composer.lock
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
			`,
		},
		{
			name:         "",
			args:         []string{"", "--format", "json", "./fixtures/locks-many/composer.lock"},
			wantExitCode: 0,
			wantStdout: `
				{
					"results": []
				}
			`,
			wantStderr: `
				Scanning dir ./fixtures/locks-many/composer.lock
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
			`,
		},
		// output format: sarif
		{
			name:         "Empty sarif output",
			args:         []string{"", "--format", "sarif", "./fixtures/locks-many/composer.lock"},
			wantExitCode: 0,
			wantStdout: `
				{
					"version": "2.1.0",
					"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
					"runs": [
						{
							"tool": {
								"driver": {
									"informationUri": "https://github.com/google/osv-scanner",
									"name": "osv-scanner",
									"rules": [
										{
											"id": "vulnerable-packages",
											"shortDescription": {
												"text": "This manifest file contains one or more vulnerable packages."
											}
										}
									]
								}
							},
							"results": []
						}
					]
				}
			`,
			wantStderr: `
				Scanning dir ./fixtures/locks-many/composer.lock
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
			`,
		},
		{
			name:         "Sarif with vulns",
			args:         []string{"", "--format", "sarif", "--config", "./fixtures/osv-scanner-empty-config.toml", "./fixtures/locks-many/package-lock.json"},
			wantExitCode: 1,
			wantStdout: `
				{
					"version": "2.1.0",
					"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
					"runs": [
						{
							"tool": {
								"driver": {
									"informationUri": "https://github.com/google/osv-scanner",
									"name": "osv-scanner",
									"rules": [
										{
											"id": "vulnerable-packages",
											"shortDescription": {
												"text": "This manifest file contains one or more vulnerable packages."
											}
										}
									]
								}
							},
							"artifacts": [
								{
									"location": {
										"uri": "fixtures/locks-many/package-lock.json"
									},
									"length": -1
								}
							],
							"results": [
								{
									"ruleId": "vulnerable-packages",
									"ruleIndex": 0,
									"level": "warning",
									"message": {
										"text": "+-----------+-------------------------------------+------+-----------------+---------------+\n| PACKAGE \u0026nbsp; | VULNERABILITY ID \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp;| CVSS | CURRENT VERSION | FIXED VERSION |\n+-----------+-------------------------------------+------+-----------------+---------------+\n| ansi-html | https://osv.dev/GHSA-whgm-jr23-g3j9 | 7.5 \u0026nbsp;| 0.0.1 \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp; | 0.0.8 \u0026nbsp; \u0026nbsp; \u0026nbsp; \u0026nbsp; |\n+-----------+-------------------------------------+------+-----------------+---------------+"
									},
									"locations": [
										{
											"physicalLocation": {
												"artifactLocation": {
													"uri": "fixtures/locks-many/package-lock.json"
												}
											}
										}
									]
								}
							]
						}
					]
				}
			`,
			wantStderr: `
				Scanning dir ./fixtures/locks-many/package-lock.json
				Scanned %%/fixtures/locks-many/package-lock.json file and found 1 package
			`,
		},
		// output format: markdown table
		{
			name:         "",
			args:         []string{"", "--format", "markdown", "./fixtures/locks-many/composer.lock"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-many/composer.lock
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
				No issues found
			`,
			wantStderr: "",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testCli(t, tt)
		})
	}
}

func TestRun_LockfileWithExplicitParseAs(t *testing.T) {
	t.Parallel()

	tests := []cliTestCase{
		// unsupported parse-as
		{
			name:         "",
			args:         []string{"", "-L", "my-file:./fixtures/locks-many/composer.lock"},
			wantExitCode: 127,
			wantStdout:   "",
			wantStderr: `
				could not determine extractor, requested my-file
			`,
		},
		// empty is default
		{
			name: "",
			args: []string{
				"",
				"-L",
				":" + filepath.FromSlash("./fixtures/locks-many/composer.lock"),
			},
			wantExitCode: 0,
			wantStdout: `
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
				No issues found
			`,
			wantStderr: "",
		},
		// empty works as an escape (no fixture because it's not valid on Windows)
		{
			name: "",
			args: []string{
				"",
				"-L",
				":" + filepath.FromSlash("./path/to/my:file"),
			},
			wantExitCode: 127,
			wantStdout:   "",
			wantStderr: `
				open %%/path/to/my:file: no such file or directory
			`,
		},
		{
			name: "",
			args: []string{
				"",
				"-L",
				":" + filepath.FromSlash("./path/to/my:project/package-lock.json"),
			},
			wantExitCode: 127,
			wantStdout:   "",
			wantStderr: `
				open %%/path/to/my:project/package-lock.json: no such file or directory
			`,
		},
		// when an explicit parse-as is given, it's applied to that file
		{
			name: "",
			args: []string{
				"",
				"-L",
				"package-lock.json:" + filepath.FromSlash("./fixtures/locks-insecure/my-package-lock.json"),
				filepath.FromSlash("./fixtures/locks-insecure"),
			},
			wantExitCode: 1,
			wantStdout: `
				Scanned %%/fixtures/locks-insecure/my-package-lock.json file as a package-lock.json and found 1 package
				Scanning dir ./fixtures/locks-insecure
				Scanned %%/fixtures/locks-insecure/composer.lock file and found 0 packages
				+-------------------------------------+------+-----------+-----------+---------+----------------------------------------------+
				| OSV URL                             | CVSS | ECOSYSTEM | PACKAGE   | VERSION | SOURCE                                       |
				+-------------------------------------+------+-----------+-----------+---------+----------------------------------------------+
				| https://osv.dev/GHSA-whgm-jr23-g3j9 | 7.5  | npm       | ansi-html | 0.0.1   | fixtures/locks-insecure/my-package-lock.json |
				+-------------------------------------+------+-----------+-----------+---------+----------------------------------------------+
			`,
			wantStderr: "",
		},
		// multiple, + output order is deterministic
		{
			name: "",
			args: []string{
				"",
				"-L", "package-lock.json:" + filepath.FromSlash("./fixtures/locks-insecure/my-package-lock.json"),
				"-L", "yarn.lock:" + filepath.FromSlash("./fixtures/locks-insecure/my-yarn.lock"),
				filepath.FromSlash("./fixtures/locks-insecure"),
			},
			wantExitCode: 1,
			wantStdout: `
				Scanned %%/fixtures/locks-insecure/my-package-lock.json file as a package-lock.json and found 1 package
				Scanned %%/fixtures/locks-insecure/my-yarn.lock file as a yarn.lock and found 1 package
				Scanning dir ./fixtures/locks-insecure
				Scanned %%/fixtures/locks-insecure/composer.lock file and found 0 packages
				+-------------------------------------+------+-----------+-----------+---------+----------------------------------------------+
				| OSV URL                             | CVSS | ECOSYSTEM | PACKAGE   | VERSION | SOURCE                                       |
				+-------------------------------------+------+-----------+-----------+---------+----------------------------------------------+
				| https://osv.dev/GHSA-whgm-jr23-g3j9 | 7.5  | npm       | ansi-html | 0.0.1   | fixtures/locks-insecure/my-package-lock.json |
				| https://osv.dev/GHSA-whgm-jr23-g3j9 | 7.5  | npm       | ansi-html | 0.0.1   | fixtures/locks-insecure/my-yarn.lock         |
				+-------------------------------------+------+-----------+-----------+---------+----------------------------------------------+
			`,
			wantStderr: "",
		},
		{
			name: "",
			args: []string{
				"",
				"-L", "yarn.lock:" + filepath.FromSlash("./fixtures/locks-insecure/my-yarn.lock"),
				"-L", "package-lock.json:" + filepath.FromSlash("./fixtures/locks-insecure/my-package-lock.json"),
				filepath.FromSlash("./fixtures/locks-insecure"),
			},
			wantExitCode: 1,
			wantStdout: `
				Scanned %%/fixtures/locks-insecure/my-yarn.lock file as a yarn.lock and found 1 package
				Scanned %%/fixtures/locks-insecure/my-package-lock.json file as a package-lock.json and found 1 package
				Scanning dir ./fixtures/locks-insecure
				Scanned %%/fixtures/locks-insecure/composer.lock file and found 0 packages
				+-------------------------------------+------+-----------+-----------+---------+----------------------------------------------+
				| OSV URL                             | CVSS | ECOSYSTEM | PACKAGE   | VERSION | SOURCE                                       |
				+-------------------------------------+------+-----------+-----------+---------+----------------------------------------------+
				| https://osv.dev/GHSA-whgm-jr23-g3j9 | 7.5  | npm       | ansi-html | 0.0.1   | fixtures/locks-insecure/my-package-lock.json |
				| https://osv.dev/GHSA-whgm-jr23-g3j9 | 7.5  | npm       | ansi-html | 0.0.1   | fixtures/locks-insecure/my-yarn.lock         |
				+-------------------------------------+------+-----------+-----------+---------+----------------------------------------------+
			`,
			wantStderr: "",
		},
		// files that error on parsing stop parsable files from being checked
		{
			name: "",
			args: []string{
				"",
				"-L",
				"Cargo.lock:" + filepath.FromSlash("./fixtures/locks-insecure/my-package-lock.json"),
				filepath.FromSlash("./fixtures/locks-insecure"),
				filepath.FromSlash("./fixtures/locks-many"),
			},
			wantExitCode: 127,
			wantStdout:   "",
			wantStderr: `
				(extracting as Cargo.lock) could not extract from %%/fixtures/locks-insecure/my-package-lock.json: toml: line 1: expected '.' or '=', but got '{' instead
			`,
		},
		// parse-as takes priority, even if it's wrong
		{
			name: "",
			args: []string{
				"",
				"-L",
				"package-lock.json:" + filepath.FromSlash("./fixtures/locks-many/yarn.lock"),
			},
			wantExitCode: 127,
			wantStdout:   "",
			wantStderr: `
				(extracting as package-lock.json) could not extract from %%/fixtures/locks-many/yarn.lock: invalid character '#' looking for beginning of value
			`,
		},
		// "apk-installed" is supported
		{
			name: "",
			args: []string{
				"",
				"-L",
				"apk-installed:" + filepath.FromSlash("./fixtures/locks-many/installed"),
			},
			wantExitCode: 0,
			wantStdout: `
				Scanned %%/fixtures/locks-many/installed file as a apk-installed and found 1 package
				No issues found
			`,
			wantStderr: "",
		},
		// "dpkg-status" is supported
		{
			name: "",
			args: []string{
				"",
				"-L",
				"dpkg-status:" + filepath.FromSlash("./fixtures/locks-many/status"),
			},
			wantExitCode: 0,
			wantStdout: `
				Scanned %%/fixtures/locks-many/status file as a dpkg-status and found 1 package
				No issues found
			`,
			wantStderr: "",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testCli(t, tt)
		})
	}
}

func TestRun_LocalDatabases(t *testing.T) {
	t.Parallel()

	tests := []cliTestCase{
		// one specific supported lockfile
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "./fixtures/locks-many/composer.lock"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-many/composer.lock
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
				Loaded Packagist local db from %%/osv-scanner/Packagist/all.zip
				No issues found
			`,
			wantStderr: "",
		},
		// one specific supported sbom with vulns
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "--config=./fixtures/osv-scanner-empty-config.toml", "./fixtures/sbom-insecure/postgres-stretch.cdx.xml"},
			wantExitCode: 1,
			wantStdout: `
				Scanning dir ./fixtures/sbom-insecure/postgres-stretch.cdx.xml
				Scanned %%/fixtures/sbom-insecure/postgres-stretch.cdx.xml as CycloneDX SBOM and found 136 packages
				Loaded Debian local db from %%/osv-scanner/Debian/all.zip
				Loaded Go local db from %%/osv-scanner/Go/all.zip
				Loaded OSS-Fuzz local db from %%/osv-scanner/OSS-Fuzz/all.zip
				+-------------------------------------+------+-----------+--------------------------------+------------------------------------+-------------------------------------------------+
				| OSV URL                             | CVSS | ECOSYSTEM | PACKAGE                        | VERSION                            | SOURCE                                          |
				+-------------------------------------+------+-----------+--------------------------------+------------------------------------+-------------------------------------------------+
				| https://osv.dev/GHSA-f3fp-gc8g-vw66 | 5.9  | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-g2j6-57v7-gm8c | 6.1  | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-m8cg-xc2p-r3fc | 2.5  | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-v95c-p5hm-xq8f | 6    | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-vpvm-3wq2-2wvm | 7    | Go        | github.com/opencontainers/runc | v1.0.1                             | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				| https://osv.dev/GHSA-p782-xgp4-8hr8 | 5.3  | Go        | golang.org/x/sys               | v0.0.0-20210817142637-7d9622a276b7 | fixtures/sbom-insecure/postgres-stretch.cdx.xml |
				+-------------------------------------+------+-----------+--------------------------------+------------------------------------+-------------------------------------------------+
			`,
			wantStderr: "",
		},
		// one specific unsupported lockfile
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "./fixtures/locks-many/not-a-lockfile.toml"},
			wantExitCode: 128,
			wantStdout: `
				Scanning dir ./fixtures/locks-many/not-a-lockfile.toml
			`,
			wantStderr: `
				No package sources found, --help for usage information.
			`,
		},
		// all supported lockfiles in the directory should be checked
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "./fixtures/locks-many"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-many
				Scanned %%/fixtures/locks-many/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-many/alpine.cdx.xml as CycloneDX SBOM and found 15 packages
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
				Scanned %%/fixtures/locks-many/package-lock.json file and found 1 package
				Scanned %%/fixtures/locks-many/yarn.lock file and found 1 package
				Loaded RubyGems local db from %%/osv-scanner/RubyGems/all.zip
				Loaded Alpine local db from %%/osv-scanner/Alpine/all.zip
				Loaded Packagist local db from %%/osv-scanner/Packagist/all.zip
				Loaded npm local db from %%/osv-scanner/npm/all.zip
				Loaded filter from: %%/fixtures/locks-many/osv-scanner.toml
				GHSA-whgm-jr23-g3j9 has been filtered out because: Test manifest file
				Filtered 1 vulnerability from output
				No issues found
			`,
			wantStderr: "",
		},
		// all supported lockfiles in the directory should be checked
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "./fixtures/locks-many-with-invalid"},
			wantExitCode: 127,
			wantStdout: `
				Scanning dir ./fixtures/locks-many-with-invalid
				Scanned %%/fixtures/locks-many-with-invalid/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-many-with-invalid/yarn.lock file and found 1 package
				Loaded RubyGems local db from %%/osv-scanner/RubyGems/all.zip
				Loaded npm local db from %%/osv-scanner/npm/all.zip
			`,
			wantStderr: `
				Attempted to scan lockfile but failed: %%/fixtures/locks-many-with-invalid/composer.lock
			`,
		},
		// only the files in the given directories are checked by default (no recursion)
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "./fixtures/locks-one-with-nested"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-one-with-nested
				Scanned %%/fixtures/locks-one-with-nested/yarn.lock file and found 1 package
				Loaded npm local db from %%/osv-scanner/npm/all.zip
				No issues found
			`,
			wantStderr: "",
		},
		// nested directories are checked when `--recursive` is passed
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "--recursive", "./fixtures/locks-one-with-nested"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-one-with-nested
				Scanned %%/fixtures/locks-one-with-nested/nested/composer.lock file and found 1 package
				Scanned %%/fixtures/locks-one-with-nested/yarn.lock file and found 1 package
				Loaded Packagist local db from %%/osv-scanner/Packagist/all.zip
				Loaded npm local db from %%/osv-scanner/npm/all.zip
				No issues found
			`,
			wantStderr: "",
		},
		// .gitignored files
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "--recursive", "./fixtures/locks-gitignore"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-gitignore
				Scanned %%/fixtures/locks-gitignore/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/subdir/yarn.lock file and found 1 package
				Loaded RubyGems local db from %%/osv-scanner/RubyGems/all.zip
				Loaded npm local db from %%/osv-scanner/npm/all.zip
				No issues found
			`,
			wantStderr: "",
		},
		// ignoring .gitignore
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "--recursive", "--no-ignore", "./fixtures/locks-gitignore"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-gitignore
				Scanned %%/fixtures/locks-gitignore/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/composer.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/ignored/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/ignored/yarn.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/subdir/Gemfile.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/subdir/composer.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/subdir/yarn.lock file and found 1 package
				Scanned %%/fixtures/locks-gitignore/yarn.lock file and found 1 package
				Loaded RubyGems local db from %%/osv-scanner/RubyGems/all.zip
				Loaded Packagist local db from %%/osv-scanner/Packagist/all.zip
				Loaded npm local db from %%/osv-scanner/npm/all.zip
				No issues found
			`,
			wantStderr: "",
		},
		// output with json
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "--json", "./fixtures/locks-many/composer.lock"},
			wantExitCode: 0,
			wantStdout: `
				{
					"results": []
				}
			`,
			wantStderr: `
				Scanning dir ./fixtures/locks-many/composer.lock
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
				Loaded Packagist local db from %%/osv-scanner/Packagist/all.zip
			`,
		},
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "--format", "json", "./fixtures/locks-many/composer.lock"},
			wantExitCode: 0,
			wantStdout: `
				{
					"results": []
				}
			`,
			wantStderr: `
				Scanning dir ./fixtures/locks-many/composer.lock
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
				Loaded Packagist local db from %%/osv-scanner/Packagist/all.zip
			`,
		},
		// output format: markdown table
		{
			name:         "",
			args:         []string{"", "--experimental-local-db", "--format", "markdown", "./fixtures/locks-many/composer.lock"},
			wantExitCode: 0,
			wantStdout: `
				Scanning dir ./fixtures/locks-many/composer.lock
				Scanned %%/fixtures/locks-many/composer.lock file and found 1 package
				Loaded Packagist local db from %%/osv-scanner/Packagist/all.zip
				No issues found
			`,
			wantStderr: "",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testDir, cleanupTestDir := createTestDir(t)
			defer cleanupTestDir()

			old := tt.args

			tt.args = []string{"", "--experimental-local-db-path", testDir}
			tt.args = append(tt.args, old[1:]...)

			// run each test twice since they should provide the same output,
			// and the second run should be fast as the db is already available
			testCli(t, tt)
			testCli(t, tt)
		})
	}
}

func TestMain(m *testing.M) {
	// ensure a git repository doesn't already exist in the fixtures directory,
	// in case we didn't get a chance to clean-up properly in the last run
	os.RemoveAll("./fixtures/.git")

	// Temporarily make the fixtures folder a git repository to prevent gitignore files messing with tests.
	_, err := git.PlainInit("./fixtures", false)
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll("./fixtures/.git")
	os.Exit(code)
}
