package hqlint

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The five areas, and the files each area must carry.
var areas = []string{
	"00-GENESIS",
	"01-RESEARCH",
	"02-DESIGN",
	"03-IMPLEMENTATION",
	"04-JOURNEY",
}

var genesisFiles = []string{"README.md", "vision.md", "constitution.md", "how-we-work.md"}

var (
	legalStates    = map[string]bool{"active": true, "graduated": true, "abandoned": true}
	terminalStates = map[string]bool{"graduated": true, "abandoned": true}
)

var (
	episodeRE = regexp.MustCompile(`^\d{4}-[a-z0-9-]+\.md$`)
	stateRE   = regexp.MustCompile(`(?m)^\*\*State:\*\* *(\S+)`)
	linkRE    = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
)

// nonEpisode names the non-episode files that legitimately live in 04-JOURNEY.
var nonEpisode = map[string]bool{"README.md": true, "TEMPLATE.md": true}

// repoRoot locates the soulrealm repository root relative to this source file
// (internal/hqlint/hqlint_test.go), so the checks read the same tree no matter
// what working directory `go test` is invoked from.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("hqlint: cannot locate source file via runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func hqDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "hq")
}

// mustFile fails the test when path is not an existing regular file.
func mustFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		t.Errorf("missing required file: %s", path)
	}
}

// episodes returns the sorted names of the episode files in 04-JOURNEY (every
// file except the README and TEMPLATE).
func episodes(t *testing.T, hq string) []string {
	t.Helper()
	dir := filepath.Join(hq, "04-JOURNEY")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("hqlint: cannot read %s: %v", dir, err)
	}
	var eps []string
	for _, e := range entries {
		if e.IsDir() || nonEpisode[e.Name()] {
			continue
		}
		eps = append(eps, e.Name())
	}
	sort.Strings(eps)
	return eps
}

func TestHQAreasExistWithReadmes(t *testing.T) {
	hq := hqDir(t)
	mustFile(t, filepath.Join(hq, "README.md"))
	for _, area := range areas {
		mustFile(t, filepath.Join(hq, area, "README.md"))
	}
	for _, name := range genesisFiles {
		mustFile(t, filepath.Join(hq, "00-GENESIS", name))
	}
	mustFile(t, filepath.Join(hq, "01-RESEARCH", "TEMPLATE.md"))
	mustFile(t, filepath.Join(hq, "04-JOURNEY", "TEMPLATE.md"))
}

func TestResearchTopicsHaveLegalNonterminalState(t *testing.T) {
	hq := hqDir(t)
	researchDir := filepath.Join(hq, "01-RESEARCH")
	entries, err := os.ReadDir(researchDir)
	if err != nil {
		t.Fatalf("hqlint: cannot read %s: %v", researchDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		topic := e.Name()
		readme := filepath.Join(researchDir, topic, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			t.Errorf("%s: research topic without README.md", topic)
			continue
		}
		text := string(data)
		if !strings.HasPrefix(strings.TrimSpace(text), "# ") {
			t.Errorf("%s: README lacks a title", topic)
		}
		if !strings.Contains(text, "## Abstract") {
			t.Errorf("%s: README lacks an Abstract section", topic)
		}
		m := stateRE.FindStringSubmatch(text)
		if m == nil {
			t.Errorf("%s: README lacks a '**State:** ...' line", topic)
			continue
		}
		state := m[1]
		if !legalStates[state] {
			t.Errorf("%s: illegal state %q", topic, state)
			continue
		}
		if terminalStates[state] {
			t.Errorf("%s: state %q is terminal but the folder lingers — "+
				"/research-graduate removes the topic folder on every outcome", topic, state)
		}
	}
}

func TestJourneyEpisodesNumberedContiguously(t *testing.T) {
	hq := hqDir(t)
	var nums []int
	seen := map[int]bool{}
	for _, name := range episodes(t, hq) {
		if !episodeRE.MatchString(name) {
			t.Errorf("file in hq/04-JOURNEY is not an NNNN-slug.md episode: %s", name)
			continue
		}
		n, err := strconv.Atoi(name[:4])
		if err != nil {
			t.Errorf("episode %s has a non-numeric prefix", name)
			continue
		}
		if seen[n] {
			t.Errorf("duplicate episode number: %04d", n)
		}
		seen[n] = true
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for i, n := range nums {
		if n != i+1 {
			t.Errorf("episode numbers not contiguous from 0001: got %v", nums)
			break
		}
	}
}

func TestJourneyEpisodesAreIndexed(t *testing.T) {
	hq := hqDir(t)
	idx, err := os.ReadFile(filepath.Join(hq, "04-JOURNEY", "README.md"))
	if err != nil {
		t.Fatalf("hqlint: cannot read the journey index: %v", err)
	}
	index := string(idx)
	for _, name := range episodes(t, hq) {
		if !strings.Contains(index, name) {
			t.Errorf("episode missing from the hq/04-JOURNEY/README.md index: %s", name)
		}
	}
}

// TestEpisodesRecordReversalCondition holds every episode to soulrealm's rule
// (hq/04-JOURNEY/TEMPLATE.md): the Reversal condition line is required on all
// episodes, not only later ones — soulrealm never had a pre-split exemption.
func TestEpisodesRecordReversalCondition(t *testing.T) {
	hq := hqDir(t)
	for _, name := range episodes(t, hq) {
		data, err := os.ReadFile(filepath.Join(hq, "04-JOURNEY", name))
		if err != nil {
			t.Errorf("cannot read episode %s: %v", name, err)
			continue
		}
		if !strings.Contains(string(data), "Reversal condition:") {
			t.Errorf("episode without the required 'Reversal condition:' line: %s "+
				"(see hq/04-JOURNEY/TEMPLATE.md)", name)
		}
	}
}

func TestConstitutionSymlinkResolvesToGenesis(t *testing.T) {
	root := repoRoot(t)
	link := filepath.Join(root, ".specify", "memory", "constitution.md")

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf(".specify/memory/constitution.md is missing: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal(".specify/memory/constitution.md must be a symlink into hq/00-GENESIS")
	}

	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("dangling symlink — speckit would re-copy its template over it: %v", err)
	}
	canonical, err := filepath.EvalSymlinks(filepath.Join(root, "hq", "00-GENESIS", "constitution.md"))
	if err != nil {
		t.Fatalf("canonical constitution missing: %v", err)
	}
	if resolved != canonical {
		t.Fatalf("symlink resolves to %s, not %s", resolved, canonical)
	}

	data, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("cannot read the constitution: %v", err)
	}
	if !strings.Contains(string(data), "# Soulrealm Constitution") {
		t.Error("constitution missing the expected '# Soulrealm Constitution' heading")
	}
}

func TestHQRelativeLinksResolve(t *testing.T) {
	hq := hqDir(t)
	var broken []string
	err := filepath.WalkDir(hq, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range linkRE.FindAllStringSubmatch(string(data), -1) {
			target := m[1]
			if strings.HasPrefix(target, "http://") ||
				strings.HasPrefix(target, "https://") ||
				strings.HasPrefix(target, "mailto:") ||
				strings.HasPrefix(target, "#") {
				continue
			}
			if i := strings.Index(target, "#"); i >= 0 {
				target = target[:i]
			}
			if target == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), target)); err != nil {
				rel, relErr := filepath.Rel(hq, path)
				if relErr != nil {
					rel = path
				}
				broken = append(broken, rel+" -> "+m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("hqlint: walking hq/: %v", err)
	}
	if len(broken) > 0 {
		t.Errorf("broken relative markdown links inside hq/:\n%s", strings.Join(broken, "\n"))
	}
}
