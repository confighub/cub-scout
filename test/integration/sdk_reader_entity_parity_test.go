//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/hubread"
)

// #758: the reads of one thing, by cub and by the SDK reader, on a real
// ConfigHub server: a Unit's data, each of its revisions and their data, a
// View by its ID, and a change order. A step this server does not allow is
// recorded, and what depends on it is skipped and said to be.
func TestSDKReaderMatchesCubOnEntitiesAndData(t *testing.T) {
	if os.Getenv(disposableConfigHubEnv) != "1" {
		t.Skipf("writes a space, Units, a View and a change order; set %s=1 only for a disposable ConfigHub server", disposableConfigHubEnv)
	}
	skipIfNotConnected(t)
	contextOut, err := exec.Command("cub", "context", "get").CombinedOutput()
	if err != nil {
		t.Fatalf("cub context get: %v: %s", err, contextOut)
	}
	if strings.Contains(strings.ToLower(string(contextOut)), "confighub.com") {
		t.Fatalf("refusing to run: the cub context names a hosted ConfigHub server, not a disposable one")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	reader, err := hubread.Resolve(ctx, hubread.Options{UserAgent: "cub-scout-integration"})
	if err != nil {
		t.Fatalf("resolve the credential cub uses: %v", err)
	}
	if host := serverHost(reader.Server()); host != "localhost" && host != "127.0.0.1" && host != "::1" {
		t.Fatalf("refusing to run: the resolved server %q is not on this machine", host)
	}

	out := os.Getenv(sdkParityOutEnv)
	if out == "" {
		out = t.TempDir()
	}
	out = filepath.Join(out, "entities")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(out, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	space := fmt.Sprintf("scout-sdk-entities-%d", time.Now().UnixNano())
	if _, err := cubStdoutOnly("space", "create", space); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTestSpace(t, space) })
	manifest := func(name, value string) (string, []byte) {
		path := filepath.Join(t.TempDir(), name+".yaml")
		body := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: " + name + "\n  namespace: default\ndata:\n  key: " + value + "\n")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return path, body
	}
	const unit = "entity-unit"
	firstPath, first := manifest(unit, "one")
	if _, err := cubStdoutOnly("unit", "create", "--space", space, unit, firstPath); err != nil {
		t.Fatal(err)
	}
	secondPath, second := manifest(unit, "two")
	if _, err := cubStdoutOnly("unit", "update", "--space", space, unit, secondPath); err != nil {
		t.Fatal(err)
	}
	gotUnit, err := cubStdoutOnly("unit", "get", unit, "--space", space, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Unit struct {
			SpaceID, DataHash string
			HeadRevisionNum   int64
		}
	}
	if err := json.Unmarshal(gotUnit, &envelope); err != nil || envelope.Unit.SpaceID == "" {
		t.Fatalf("the new Unit could not be read: %v: %s", err, gotUnit)
	}
	spaceID, head := envelope.Unit.SpaceID, envelope.Unit.HeadRevisionNum

	equal, notes := map[string]bool{}, map[string]string{}
	compare := func(name string, cubArgs []string, read func(space string) ([]byte, error)) []byte {
		t.Helper()
		viaCub, err := cubStdoutOnly(cubArgs...)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return nil
		}
		keep(name+".cub", viaCub)
		for how, named := range map[string]string{"slug": space, "id": spaceID} {
			viaSDK, err := read(named)
			if err != nil {
				t.Errorf("%s through the SDK, space by %s: %v (kind %s)", name, how, err, hubread.KindOf(err))
				continue
			}
			if how == "slug" {
				keep(name+".sdk", viaSDK)
			}
			equal[name+"-by-"+how] = bytes.Equal(viaCub, viaSDK)
			if !bytes.Equal(viaCub, viaSDK) {
				keep(name+".sdk-by-"+how, viaSDK)
				t.Errorf("%s, space by %s: the SDK reader's bytes are not what cub printed: %d against %d; both are kept in %s", name, how, len(viaSDK), len(viaCub), out)
			}
		}
		return viaCub
	}
	hashOf := func(data []byte) string {
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}

	// The Unit's data: what was uploaded last, and its hash is the one the
	// Unit reports. The reader's own check rests on that.
	data := compare("unit-data", []string{"unit", "data", unit, "--space", space},
		func(s string) ([]byte, error) { return reader.UnitData(ctx, s, unit) })
	notes["unit data is the file uploaded last"] = fmt.Sprint(bytes.Equal(data, second))
	notes["unit DataHash is the SHA-256 of its data"] = fmt.Sprint(envelope.Unit.DataHash == hashOf(data))
	if envelope.Unit.DataHash != hashOf(data) {
		t.Errorf("the Unit reports DataHash %q and its data hashes to %q", envelope.Unit.DataHash, hashOf(data))
	}

	// Each revision, and its data. The head is the second upload; which
	// earlier number holds the first is the server's business, so every
	// number up to the head is read.
	notes["head revision"] = fmt.Sprint(head)
	sawFirst := false
	for number := int64(1); number <= head; number++ {
		label := fmt.Sprintf("revision-%d", number)
		text := fmt.Sprint(number)
		revision := compare(label+"-get", []string{"revision", "get", unit, text, "--space", space, "-o", "json"},
			func(s string) ([]byte, error) { return reader.RevisionJSON(ctx, s, unit, number) })
		var recorded struct {
			Revision struct {
				RevisionNum int64
				DataHash    string
			}
		}
		if err := json.Unmarshal(revision, &recorded); err != nil || recorded.Revision.RevisionNum != number {
			t.Errorf("%s: cub printed revision %d (%v)", label, recorded.Revision.RevisionNum, err)
		}
		viaCub, cubErr := cubStdoutOnly("revision", "data", unit, text, "--space", space)
		viaSDK, sdkErr := reader.RevisionData(ctx, space, unit, number)
		switch {
		case cubErr != nil && sdkErr != nil:
			// A revision with no data fails by both routes.
			notes[label+" data"] = "no data by either route: " + string(hubread.KindOf(sdkErr))
		case cubErr != nil || sdkErr != nil:
			t.Errorf("%s data: cub %v, the reader %v", label, cubErr, sdkErr)
		default:
			keep(label+"-data.cub", viaCub)
			equal[label+"-data"] = bytes.Equal(viaCub, viaSDK)
			if !bytes.Equal(viaCub, viaSDK) {
				keep(label+"-data.sdk", viaSDK)
				t.Errorf("%s data: the SDK reader's bytes are not what cub printed", label)
			}
			if recorded.Revision.DataHash != hashOf(viaCub) {
				t.Errorf("%s reports DataHash %q and its data hashes to %q", label, recorded.Revision.DataHash, hashOf(viaCub))
			}
			sawFirst = sawFirst || bytes.Equal(viaCub, first)
		}
	}
	notes["an earlier revision holds the first upload"] = fmt.Sprint(sawFirst)

	// What is not there fails by both routes and is never an empty answer.
	for name, read := range map[string]func() ([]byte, error){
		"data of a unit that does not exist":  func() ([]byte, error) { return reader.UnitData(ctx, space, "no-such-unit") },
		"a revision past the head":            func() ([]byte, error) { return reader.RevisionJSON(ctx, space, unit, head+50) },
		"data of a revision past the head":    func() ([]byte, error) { return reader.RevisionData(ctx, space, unit, head+50) },
		"data in a space that does not exist": func() ([]byte, error) { return reader.UnitData(ctx, "no-such-space-"+space, unit) },
		"a change order that does not exist":  func() ([]byte, error) { return reader.ChangeOrderJSON(ctx, space, "no-such-order") },
		"a view whose ID names nothing":       func() ([]byte, error) { return reader.ViewJSON(ctx, "*", "11111111-2222-4333-8444-555555555555") },
		"a view in a space that does not exist": func() ([]byte, error) {
			return reader.ViewJSON(ctx, "no-such-space-"+space, "11111111-2222-4333-8444-555555555555")
		},
		"a revision of a unit that is not there": func() ([]byte, error) { return reader.RevisionJSON(ctx, space, "no-such-unit", 1) },
	} {
		if got, err := read(); hubread.KindOf(err) != hubread.KindNotFound || got != nil {
			t.Errorf("%s: kind %s, want %s (%v)", name, hubread.KindOf(err), hubread.KindNotFound, err)
		}
	}
	for name, args := range map[string][]string{
		"data of a unit that does not exist": {"unit", "data", "no-such-unit", "--space", space},
		"a revision past the head":           {"revision", "get", unit, fmt.Sprint(head + 50), "--space", space, "-o", "json"},
		"a change order that does not exist": {"changeorder", "get", "no-such-order", "--space", space, "-o", "json"},
		"a view whose ID names nothing":      {"view", "get", "11111111-2222-4333-8444-555555555555", "--space", "*", "-o", "json"},
	} {
		if printed, err := cubStdoutOnly(args...); err == nil {
			t.Errorf("cub answered for %s: %s", name, printed)
		}
	}

	// A View, found by its ID with no space and with its space named.
	if _, err := cubStdoutOnly("filter", "create", "--space", space, "entity-filter", "Unit", "--where-field", "Slug LIKE 'entity-%'"); err != nil {
		notes["view"] = "no filter could be created: " + err.Error()
	} else if created, err := cubStdoutOnly("view", "create", "--space", space, "-o", "json", "entity-view", "entity-filter", "--column", "Unit.Slug"); err != nil {
		notes["view"] = "no view could be created: " + err.Error()
	} else {
		var decoded interface{}
		_ = json.Unmarshal(created, &decoded)
		viewID := findString(decoded, "ViewID")
		if viewID == "" {
			t.Errorf("the created View has no ViewID: %s", created)
		} else {
			notes["view"] = "created"
			for how, named := range map[string]string{"every space": "*", "its space by slug": space, "its space by ID": spaceID} {
				viaCub, err := cubStdoutOnly("view", "get", viewID, "--space", named, "-o", "json")
				if err != nil {
					t.Errorf("cub view get, %s: %v", how, err)
					continue
				}
				keep("view-get.cub", viaCub)
				viaSDK, err := reader.ViewJSON(ctx, named, viewID)
				if err != nil {
					t.Errorf("view get through the SDK, %s: %v (kind %s)", how, err, hubread.KindOf(err))
					continue
				}
				equal["view-get, "+how] = bytes.Equal(viaCub, viaSDK)
				if !bytes.Equal(viaCub, viaSDK) {
					keep("view-get.sdk", viaSDK)
					t.Errorf("view get, %s: the SDK reader's JSON is not what cub printed", how)
				}
			}
		}
	}

	// A change order, where this server lets a bare space have one.
	if _, err := cubStdoutOnly("changeorder", "create", "--space", space, "entity-order", "--description", "recorded for the SDK reader"); err != nil {
		notes["changeorder"] = "not created: " + err.Error()
		t.Logf("no change order could be created on this server, so that read is not compared: %v", err)
	} else {
		notes["changeorder"] = "created"
		compare("changeorder-get", []string{"changeorder", "get", "entity-order", "-o", "json", "--space", space},
			func(s string) ([]byte, error) { return reader.ChangeOrderJSON(ctx, s, "entity-order") })
	}

	summary, _ := json.MarshalIndent(map[string]interface{}{"notes": notes, "equal": equal}, "", "  ")
	keep("entities.json", append(summary, '\n'))
	t.Logf("entities and data: %s", summary)
}
