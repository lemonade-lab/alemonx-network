// Command alxman validates and edits the ALemonX plugin manifest (alx.json).
//
// Subcommands:
//
//	validate [path]        validate the manifest, exit 1 on any error
//	set-version <tag>      write the release version from a git tag
//	check-version <tag>    fail unless the manifest version matches the tag
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxManifest = 64 * 1024

var (
	idRE      = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)
	versionRE = regexp.MustCompile(`^(?:0|[1-9]\d*)(?:\.(?:0|[1-9]\d*)){1,2}(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
)

// validRuntimes mirrors alx's internal/setupplugin registry. "command" is
// accepted only inside development, exactly like the host loader.
var validRuntimes = map[string]bool{
	"":       true,
	"binary": true,
	"node":   true,
	"go":     true,
	"python": true,
}

var validAuthorizations = map[string]bool{"password": true, "native": true}
var validPickerKinds = map[string]bool{"directory": true, "file": true}
var validPlatformNames = map[string]bool{"linux": true, "darwin": true, "windows": true}

// platformKeys are the accepted entry keys: concrete platforms, family names
// and the go development runtime.
var platformKeys = map[string]bool{
	"darwin-arm64":  true,
	"darwin-amd64":  true,
	"linux-amd64":   true,
	"linux-arm64":   true,
	"windows-amd64": true,
	"linux":         true,
	"darwin":        true,
	"windows":       true,
	"go":            true,
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: alxman <validate|set-version|check-version> [args]")
	fmt.Fprintln(os.Stderr, "  validate [path]        validate the manifest (default alx.json)")
	fmt.Fprintln(os.Stderr, "  set-version <tag>      write the release version from a git tag")
	fmt.Fprintln(os.Stderr, "  check-version <tag>    fail unless the manifest version matches the tag")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "validate":
		path := "alx.json"
		if len(os.Args) > 2 {
			path = os.Args[2]
		}
		os.Exit(validate(path))
	case "set-version":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		os.Exit(setVersion(os.Args[2]))
	case "check-version":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		os.Exit(checkVersion(os.Args[2]))
	default:
		usage()
		os.Exit(2)
	}
}

func report(errors []string) int {
	if len(errors) == 0 {
		fmt.Println("alx.json OK")
		return 0
	}
	for _, err := range errors {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	return 1
}

// validate checks alx.json against the same rules as the Python validator it
// replaces. The rules now mirror alx's internal/setupplugin decodeManifest so
// CI can catch manifest drift before a release. It returns a process exit code.
func validate(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return report([]string{fmt.Sprintf("cannot read %s: %v", path, err)})
	}

	var errs []string
	if len(data) > maxManifest {
		errs = append(errs, fmt.Sprintf("manifest exceeds %d bytes", maxManifest))
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			line := bytes.Count(data[:syntaxErr.Offset], []byte{'\n'}) + 1
			return report([]string{fmt.Sprintf("invalid JSON at line %d: %v", line, err)})
		}
		return report([]string{"invalid JSON: " + err.Error()})
	}

	manifest, ok := raw.(map[string]any)
	if !ok {
		return report([]string{"manifest root must be an object"})
	}

	id, _ := manifest["id"].(string)
	if !idRE.MatchString(id) {
		errs = append(errs, fmt.Sprintf("id %q must match %s", id, idRE.String()))
	}
	if name, ok := asString(manifest["name"]); !ok || strings.TrimSpace(name) == "" {
		errs = append(errs, "name is required")
	}
	if version, ok := asString(manifest["version"]); !ok || strings.TrimSpace(version) == "" {
		errs = append(errs, "version is required")
	}

	validateNavigation(manifest, &errs)
	validateWeb(manifest, &errs)
	validateServices(manifest, &errs)
	validatePickers(manifest, &errs)
	validateStatusActions(manifest, &errs)
	validateMedia(manifest, &errs)
	validatePrivilegedOperations(manifest, &errs)
	validateRuntimeAndEntry(manifest, &errs)
	validateDevelopment(manifest, &errs)

	return report(errs)
}

func asString(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok
}

func asMap(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func asSlice(value any) ([]any, bool) {
	list, ok := value.([]any)
	return list, ok
}

func validateNavigation(manifest map[string]any, errs *[]string) {
	navigation, present := manifest["navigation"]
	if !present || navigation == nil {
		return
	}
	object, ok := asMap(navigation)
	if !ok {
		*errs = append(*errs, "navigation must be an object")
		return
	}
	if label, present := object["label"]; present {
		if _, ok := asString(label); !ok {
			*errs = append(*errs, "navigation.label must be a string")
		}
	}
	if icon, present := object["icon"]; present {
		if _, ok := asString(icon); !ok {
			*errs = append(*errs, "navigation.icon must be a string")
		}
	}
	if order, present := object["order"]; present {
		if _, ok := order.(float64); !ok {
			*errs = append(*errs, "navigation.order must be a number")
		}
	}
}

func validateWeb(manifest map[string]any, errs *[]string) {
	webVal, present := manifest["web"]
	if !present || webVal == nil {
		*errs = append(*errs, "web is required: the plugin's interface is its web UI")
		return
	}
	web, ok := asMap(webVal)
	if !ok {
		*errs = append(*errs, "web must be an object with a string root")
		return
	}
	root, ok := asString(web["root"])
	if !ok {
		*errs = append(*errs, "web must be an object with a string root")
		return
	}
	root = filepath.ToSlash(strings.TrimSpace(root))
	if root == "" || filepath.IsAbs(root) || root == ".." {
		*errs = append(*errs, "web.root must be a directory path inside the plugin")
		return
	}
	for _, component := range strings.Split(root, "/") {
		if component == ".." {
			*errs = append(*errs, "web.root must not contain ..")
			break
		}
	}
	clean := filepath.Clean(root)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		*errs = append(*errs, "web.root must be a directory path inside the plugin")
	}
}

func validateServices(manifest map[string]any, errs *[]string) {
	services, present := manifest["services"]
	if !present || services == nil {
		return
	}
	items, ok := asSlice(services)
	if !ok {
		*errs = append(*errs, "services must be an array")
		return
	}
	seen := map[string]bool{}
	for _, raw := range items {
		service, ok := asMap(raw)
		if !ok {
			*errs = append(*errs, "service entry must be an object")
			continue
		}
		id, _ := asString(service["id"])
		name, _ := asString(service["name"])
		if !idRE.MatchString(id) || strings.TrimSpace(name) == "" || seen[id] {
			*errs = append(*errs, "setup plugin service requires a unique id and name")
		}
		host, _ := asString(service["host"])
		port, _ := service["port"].(float64)
		if (host != "127.0.0.1" && host != "localhost") || port < 1 || port > 65535 {
			*errs = append(*errs, "setup plugin service must target a valid loopback port")
		}
		for _, key := range []string{"basePath", "healthPath"} {
			rawPath, present := service[key]
			if !present || rawPath == nil {
				continue
			}
			path, ok := asString(rawPath)
			if !ok {
				*errs = append(*errs, "setup plugin service path must stay within its loopback service")
				continue
			}
			path = strings.TrimSpace(path)
			if path != "" && (!strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.Contains(path, "..")) {
				*errs = append(*errs, "setup plugin service path must stay within its loopback service")
			}
		}
		if id != "" {
			seen[id] = true
		}
	}
}

func validatePickers(manifest map[string]any, errs *[]string) {
	pickers, present := manifest["systemPickers"]
	if !present || pickers == nil {
		return
	}
	items, ok := asSlice(pickers)
	if !ok {
		*errs = append(*errs, "systemPickers must be an array")
		return
	}
	seen := map[string]bool{}
	for _, raw := range items {
		picker, ok := asMap(raw)
		if !ok {
			*errs = append(*errs, "systemPickers entry must be an object")
			continue
		}
		id, _ := asString(picker["id"])
		kind, _ := asString(picker["kind"])
		if !idRE.MatchString(id) || seen[id] {
			*errs = append(*errs, "setup plugin systemPickers requires unique ids")
		}
		if !validPickerKinds[kind] {
			*errs = append(*errs, "setup plugin system picker kind must be directory or file")
		}
		if title, present := picker["title"]; present {
			text, ok := asString(title)
			if !ok || len([]rune(text)) > 120 || strings.ContainsAny(text, "\x00\r\n") {
				*errs = append(*errs, "setup plugin system picker title is invalid")
			}
		}
		if id != "" {
			seen[id] = true
		}
	}
}

func validateStatusActions(manifest map[string]any, errs *[]string) {
	actions, present := manifest["statusActions"]
	if !present || actions == nil {
		return
	}
	items, ok := asSlice(actions)
	if !ok {
		*errs = append(*errs, "statusActions must be an array")
		return
	}
	seen := map[string]bool{}
	for _, raw := range items {
		action, ok := asString(raw)
		action = strings.TrimSpace(action)
		if !ok || action == "" || len(action) > 96 || seen[action] {
			*errs = append(*errs, "setup plugin statusActions must contain unique action names")
		}
		if action != "" {
			seen[action] = true
		}
	}
}

func validateMedia(manifest map[string]any, errs *[]string) {
	media, present := manifest["media"]
	if !present || media == nil {
		return
	}
	items, ok := asSlice(media)
	if !ok {
		*errs = append(*errs, "media must be an array")
		return
	}
	seen := map[string]bool{}
	for _, raw := range items {
		item, ok := asMap(raw)
		if !ok {
			*errs = append(*errs, "media entry must be an object")
			continue
		}
		id, _ := asString(item["id"])
		action, _ := asString(item["action"])
		contentType, _ := asString(item["contentType"])
		contentType = strings.TrimSpace(strings.ToLower(contentType))
		if !idRE.MatchString(id) || action == "" || len(action) > 96 || seen[id] || contentType != "image/png" {
			*errs = append(*errs, "setup plugin media requires a unique id, action and supported content type")
		}
		if id != "" {
			seen[id] = true
		}
	}
}

func validatePrivilegedOperations(manifest map[string]any, errs *[]string) {
	operations, present := manifest["privilegedOperations"]
	if !present || operations == nil {
		return
	}
	items, ok := asSlice(operations)
	if !ok {
		*errs = append(*errs, "privilegedOperations must be an array")
		return
	}
	seen := map[string]bool{}
	for _, raw := range items {
		operation, ok := asMap(raw)
		if !ok {
			*errs = append(*errs, "privilegedOperations entry must be an object")
			continue
		}
		action, _ := asString(operation["action"])
		runnerAction, _ := asString(operation["runnerAction"])
		planAction, _ := asString(operation["planAction"])
		title, _ := asString(operation["title"])
		authorization, _ := asString(operation["authorization"])
		useLatestAudit, _ := operation["useLatestAudit"].(bool)
		action = strings.TrimSpace(action)
		runnerAction = strings.TrimSpace(runnerAction)
		planAction = strings.TrimSpace(planAction)
		title = strings.TrimSpace(title)
		authorization = strings.TrimSpace(authorization)
		if action == "" || len(action) > 96 || runnerAction == "" || len(runnerAction) > 96 || (planAction != "" && len(planAction) > 96) || title == "" || len(title) > 120 || seen[action] {
			*errs = append(*errs, "setup plugin privileged operation is invalid")
		}
		if !validAuthorizations[authorization] {
			*errs = append(*errs, "setup plugin privileged operation authorization is invalid")
		}
		if platforms, present := operation["platforms"]; !present || platforms == nil {
			*errs = append(*errs, "setup plugin privileged operation requires platforms")
		} else if list, ok := asSlice(platforms); !ok {
			*errs = append(*errs, "setup plugin privileged operation platforms are invalid")
		} else {
			seenPlatforms := map[string]bool{}
			for _, rawPlatform := range list {
				platform, _ := asString(rawPlatform)
				platform = strings.TrimSpace(platform)
				if !validPlatformNames[platform] || seenPlatforms[platform] {
					*errs = append(*errs, "setup plugin privileged operation platforms are invalid")
				}
				if platform != "" {
					seenPlatforms[platform] = true
				}
			}
		}
		if planAction != "" && useLatestAudit {
			*errs = append(*errs, "setup plugin privileged operation cannot combine a plan with latest audit")
		}
		commands, present := operation["commands"]
		commandCount := 0
		if present && commands != nil {
			if list, ok := asSlice(commands); ok {
				commandCount = len(list)
				for _, rawCommand := range list {
					command, ok := asMap(rawCommand)
					if !ok || !validCommand(command) {
						*errs = append(*errs, "setup plugin privileged operation command is invalid")
					}
				}
			} else {
				*errs = append(*errs, "setup plugin privileged operation command is invalid")
			}
		}
		if authorization == "password" && commandCount == 0 {
			*errs = append(*errs, "setup plugin password operation requires commands")
		}
		if authorization == "native" && commandCount != 0 {
			*errs = append(*errs, "setup plugin native operation must use its runner action")
		}
		if action != "" {
			seen[action] = true
		}
	}
}

func validateRuntimeAndEntry(manifest map[string]any, errs *[]string) {
	runtimeVal := ""
	if raw, present := manifest["runtime"]; present {
		var ok bool
		runtimeVal, ok = asString(raw)
		if !ok {
			runtimeVal = "\x00"
		}
	}
	if runtimeVal == "command" {
		*errs = append(*errs, "setup plugin command runtime is only available in development")
	} else if !validRuntimes[runtimeVal] {
		*errs = append(*errs, fmt.Sprintf("runtime %q must be one of binary/node/go/python", runtimeVal))
	}

	if entryVal, present := manifest["entry"]; present && entryVal != nil {
		entry, ok := asMap(entryVal)
		if !ok || len(entry) == 0 {
			*errs = append(*errs, "entry must be a non-empty object")
		} else {
			for key := range entry {
				if !platformKeys[key] {
					*errs = append(*errs, fmt.Sprintf("entry key %q is not a platform or go", key))
				}
			}
		}
	}
}

func validateDevelopment(manifest map[string]any, errs *[]string) {
	development, present := manifest["development"]
	if !present || development == nil {
		return
	}
	dev, ok := asMap(development)
	if !ok {
		*errs = append(*errs, "development must be an object")
		return
	}

	devRuntime := ""
	if raw, present := dev["runtime"]; present {
		var ok bool
		devRuntime, ok = asString(raw)
		if !ok {
			devRuntime = "\x00"
		}
	}
	if devRuntime != "" {
		runnerOK := false
		switch {
		case devRuntime == "command":
			runnerOK = validCommand(dev["command"])
		case validRuntimes[devRuntime]:
			entry, ok := asMap(dev["entry"])
			runnerOK = ok && len(entry) > 0
		}
		if !runnerOK {
			*errs = append(*errs, "setup plugin development runner is invalid")
		}
	}

	webVal, present := dev["web"]
	if present && webVal != nil {
		web, ok := asMap(webVal)
		if !ok {
			*errs = append(*errs, "development.web must be an object")
		} else {
			mode, _ := asString(web["mode"])
			if mode == "" {
				mode = "static"
			}
			if mode != "static" && mode != "dev-server" {
				*errs = append(*errs, "setup plugin development web mode must be static or dev-server")
			}
			if root, present := web["root"]; present && root != nil {
				text, ok := asString(root)
				if !ok || !validRelativeDirectory(text) {
					*errs = append(*errs, "setup plugin development web root is invalid")
				}
			}
			if !validCommand(web["build"]) {
				*errs = append(*errs, "setup plugin development build command is invalid")
			}
			if mode == "dev-server" && web["dev"] == nil {
				*errs = append(*errs, "setup plugin development dev-server requires a dev command")
			}
			if !validCommand(web["dev"]) {
				*errs = append(*errs, "setup plugin development dev command is invalid")
			}
			if healthPath, present := web["healthPath"]; present && healthPath != nil {
				text, ok := asString(healthPath)
				if !ok {
					*errs = append(*errs, "setup plugin development health path is invalid")
				} else {
					text = strings.TrimSpace(text)
					if text != "" && (!strings.HasPrefix(text, "/") || strings.Contains(text, "..") || strings.Contains(text, "\\")) {
						*errs = append(*errs, "setup plugin development health path is invalid")
					}
				}
			}
		}
	}

	if rawServices, present := dev["services"]; present && rawServices != nil {
		items, ok := asSlice(rawServices)
		if !ok {
			*errs = append(*errs, "development services must be an array")
			return
		}
		topLevel := map[string]bool{}
		if services, present := manifest["services"]; present {
			if list, ok := asSlice(services); ok {
				for _, raw := range list {
					if service, ok := asMap(raw); ok {
						if id, ok := asString(service["id"]); ok {
							topLevel[id] = true
						}
					}
				}
			}
		}
		seen := map[string]bool{}
		for _, raw := range items {
			service, ok := asMap(raw)
			if !ok {
				*errs = append(*errs, "setup plugin development service is invalid")
				continue
			}
			id, _ := asString(service["id"])
			restart, _ := asString(service["restart"])
			if !idRE.MatchString(id) || seen[id] || !topLevel[id] || (restart != "" && restart != "never" && restart != "on-failure") || !validCommand(service["command"]) {
				*errs = append(*errs, "setup plugin development service is invalid")
			}
			if id != "" {
				seen[id] = true
			}
		}
	}
}

// validCommand mirrors the host's shell-free CommandSpec validation.
func validCommand(command any) bool {
	if command == nil {
		return true
	}
	object, ok := asMap(command)
	if !ok {
		return false
	}
	program, ok := asString(object["program"])
	if !ok || strings.TrimSpace(program) == "" || strings.ContainsAny(program, "\x00\r\n/\\") {
		return false
	}
	if args, present := object["args"]; present && args != nil {
		list, ok := asSlice(args)
		if !ok {
			return false
		}
		for _, raw := range list {
			arg, ok := asString(raw)
			if !ok || strings.ContainsAny(arg, "\x00\r\n") {
				return false
			}
			if strings.Contains(arg, "${") && !strings.Contains(arg, "${ALX_PLUGIN_DEV_PORT}") {
				return false
			}
		}
	}
	return true
}

func validRelativeDirectory(value string) bool {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" || filepath.IsAbs(value) || value == ".." {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." || component == "" {
			return false
		}
	}
	return true
}

// setVersion writes the release version derived from a git tag into the
// alx.json at the repository root.
func setVersion(tag string) int {
	path, err := findManifest()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return setVersionAt(path, tag)
}

func setVersionAt(path, tag string) int {
	version := versionFromTag(tag)
	if !versionRE.MatchString(version) {
		fmt.Fprintf(os.Stderr, "invalid release tag: %s\n", tag)
		return 1
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read %s: %v\n", path, err)
		return 1
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		fmt.Fprintf(os.Stderr, "invalid JSON in %s: %v\n", path, err)
		return 1
	}

	manifest["version"] = version
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		fmt.Fprintf(os.Stderr, "cannot encode %s: %v\n", path, err)
		return 1
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cannot write %s: %v\n", path, err)
		return 1
	}
	fmt.Printf("alx.json version = %s\n", version)
	return 0
}

// checkVersion fails unless the manifest version matches the given git tag.
func checkVersion(tag string) int {
	path, err := findManifest()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return checkVersionAt(path, tag)
}

func checkVersionAt(path, tag string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read %s: %v\n", path, err)
		return 1
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		fmt.Fprintf(os.Stderr, "invalid JSON in %s: %v\n", path, err)
		return 1
	}
	actual, _ := manifest["version"].(string)
	expected := versionFromTag(tag)
	if actual != expected {
		fmt.Fprintf(os.Stderr, "manifest version %q does not match tag %s\n", actual, tag)
		return 1
	}
	fmt.Printf("manifest version %q matches tag %s\n", actual, tag)
	return 0
}

// versionFromTag strips a leading "v" from a release tag.
func versionFromTag(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "v")
}

// findManifest locates alx.json by walking up from the working directory, so
// the tool behaves the same regardless of where it is invoked from.
func findManifest() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		path := filepath.Join(dir, "alx.json")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("alx.json not found in any parent directory")
		}
		dir = parent
	}
}
