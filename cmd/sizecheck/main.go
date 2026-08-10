package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	stdhtml "html"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type budget struct {
	MaxBespokeAppJSFiles      int                 `json:"maxBespokeAppJsFiles"`
	MaxBespokeAppJSBytes      int64               `json:"maxBespokeAppJsBytes"`
	StylesGzipBytes           int64               `json:"stylesGzipBytes"`
	MaxStaticHTMLBytes        int64               `json:"maxStaticHtmlBytes"`
	MaxStaticHTMLGzipBytes    int64               `json:"maxStaticHtmlGzipBytes"`
	MaxIslandRawBytes         int64               `json:"maxIslandRawBytes"`
	MaxIslandGzipBytes        int64               `json:"maxIslandGzipBytes"`
	MaxIslandBrotliBytes      int64               `json:"maxIslandBrotliBytes"`
	MaxRouteClientGzipBytes   int64               `json:"maxRouteClientGzipBytes"`
	MaxRouteClientBrotliBytes int64               `json:"maxRouteClientBrotliBytes"`
	ColdStartGzipBytes        int64               `json:"coldStartGzipBytes"`
	ColdStartBrotliBytes      int64               `json:"coldStartBrotliBytes"`
	TotalRuntimeGzipBytes     int64               `json:"totalRuntimeGzipBytes"`
	TotalRuntimeBrotliBytes   int64               `json:"totalRuntimeBrotliBytes"`
	ServerBinaryBytes         int64               `json:"serverBinaryBytes"`
	DistributionBytes         int64               `json:"distributionBytes"`
	MinimumExportedRoutes     int                 `json:"minimumExportedRoutes"`
	ExpectedRouteCapabilities map[string][]string `json:"expectedRouteCapabilities"`
}

type runtimeSize struct {
	ColdStartGzipBytes   int64 `json:"coldStartGzipBytes"`
	ColdStartBrotliBytes int64 `json:"coldStartBrotliBytes"`
	TotalGzipBytes       int64 `json:"totalGzipBytes"`
	TotalBrotliBytes     int64 `json:"totalBrotliBytes"`
}

type exportManifest struct {
	Routes []struct {
		Path         string                     `json:"path"`
		File         string                     `json:"file"`
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	} `json:"routes"`
}

type staticHTMLSize struct {
	RawPath  string
	GzipPath string
	Raw      int64
	Gzip     int64
}

type compressedArtifact struct {
	Path   string
	Raw    int64
	Gzip   int64
	Brotli int64
}

type routeTransfer struct {
	Path   string
	Gzip   int64
	Brotli int64
	Assets int
}

type routeSource struct {
	Path string
	HTML []byte
}

var gosxAssetPattern = regexp.MustCompile(`/gosx/assets/(?:runtime|islands)/[A-Za-z0-9._-]+`)
var gosxManifestPattern = regexp.MustCompile(`(?s)<script[^>]+id="gosx-manifest"[^>]*>(.*?)</script>`)

func main() {
	root := flag.String("root", ".", "Rostrum project root")
	flag.Parse()

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fatalf("resolve root: %v", err)
	}

	var limits budget
	readJSON(filepath.Join(absRoot, "size-budget.json"), &limits)

	runtime := readRuntimeSize(absRoot)
	bespokeJSFiles, bespokeJSBytes := bespokeBrowserJavaScript(absRoot)
	stylesGzip := gzipFileSize(filepath.Join(absRoot, "public", "styles.css"))
	staticHTML := largestStaticHTML(filepath.Join(absRoot, "dist", "static"))
	largestIsland := largestIslandProgram(filepath.Join(absRoot, "dist", "assets", "islands"))
	routes := representativeRouteSources(absRoot, limits.ExpectedRouteCapabilities)
	transfers := routeClientTransfers(absRoot, routes)
	serverBinary := fileSize(filepath.Join(absRoot, "dist", "server", "app"))
	distribution := directorySize(filepath.Join(absRoot, "dist"))

	failed := false
	failed = checkMaximum("bespoke app JS files", int64(bespokeJSFiles), int64(limits.MaxBespokeAppJSFiles)) || failed
	failed = checkMaximum("bespoke app JS bytes", bespokeJSBytes, limits.MaxBespokeAppJSBytes) || failed
	failed = checkLimit("styles.css gzip", stylesGzip, limits.StylesGzipBytes) || failed
	failed = checkLimit("largest static HTML", staticHTML.Raw, limits.MaxStaticHTMLBytes) || failed
	failed = checkLimit("largest static HTML gzip", staticHTML.Gzip, limits.MaxStaticHTMLGzipBytes) || failed
	failed = checkLimit("largest island raw", largestIsland.Raw, limits.MaxIslandRawBytes) || failed
	failed = checkLimit("largest island gzip", largestIsland.Gzip, limits.MaxIslandGzipBytes) || failed
	failed = checkLimit("largest island brotli", largestIsland.Brotli, limits.MaxIslandBrotliBytes) || failed
	failed = checkLimit("max route client gzip", largestRouteGzip(transfers), limits.MaxRouteClientGzipBytes) || failed
	failed = checkLimit("max route client brotli", largestRouteBrotli(transfers), limits.MaxRouteClientBrotliBytes) || failed
	failed = checkLimit("GoSX cold-start gzip", runtime.ColdStartGzipBytes, limits.ColdStartGzipBytes) || failed
	failed = checkLimit("GoSX cold-start brotli", runtime.ColdStartBrotliBytes, limits.ColdStartBrotliBytes) || failed
	failed = checkLimit("GoSX total runtime gzip", runtime.TotalGzipBytes, limits.TotalRuntimeGzipBytes) || failed
	failed = checkLimit("GoSX total runtime brotli", runtime.TotalBrotliBytes, limits.TotalRuntimeBrotliBytes) || failed
	failed = checkLimit("server binary", serverBinary, limits.ServerBinaryBytes) || failed
	failed = checkLimit("distribution", distribution, limits.DistributionBytes) || failed

	fmt.Printf("largest static route         %s\n", relativePath(absRoot, staticHTML.RawPath))
	if staticHTML.GzipPath != staticHTML.RawPath {
		fmt.Printf("largest compressed route     %s\n", relativePath(absRoot, staticHTML.GzipPath))
	}
	fmt.Printf("largest island program       %s\n", relativePath(absRoot, largestIsland.Path))
	for _, transfer := range transfers {
		fmt.Printf("route client %-15s %8d gzip / %8d brotli (%d assets)\n", transfer.Path, transfer.Gzip, transfer.Brotli, transfer.Assets)
	}
	if checkRouteCapabilities(absRoot, routes, limits) {
		failed = true
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("size budget                  PASS")
}

func readRuntimeSize(root string) runtimeSize {
	cli := strings.TrimSpace(os.Getenv("GOSX"))
	if cli == "" {
		cli = "gosx"
	}
	command := exec.Command(cli, "size", "--json", "dist")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		fatalf("run gosx size (build with `make build` first): %v", err)
	}
	var report runtimeSize
	if err := json.Unmarshal(output, &report); err != nil {
		fatalf("decode gosx size: %v", err)
	}
	return report
}

func readJSON(path string, target any) {
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		fatalf("decode %s: %v", path, err)
	}
}

func gzipFileSize(path string) int64 {
	file, err := os.Open(path)
	if err != nil {
		fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	counter := &countWriter{}
	writer, err := gzip.NewWriterLevel(counter, gzip.BestCompression)
	if err != nil {
		fatalf("create gzip writer: %v", err)
	}
	if _, err := io.Copy(writer, file); err != nil {
		fatalf("compress %s: %v", path, err)
	}
	if err := writer.Close(); err != nil {
		fatalf("close gzip writer: %v", err)
	}
	return counter.bytes
}

func bespokeBrowserJavaScript(root string) (int, int64) {
	paths := make(map[string]bool)
	collect := func(scanRoot string, include func(string) bool) {
		if _, err := os.Stat(scanRoot); os.IsNotExist(err) {
			return
		} else if err != nil {
			fatalf("stat %s: %v", scanRoot, err)
		}
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && include(path) {
				paths[path] = true
			}
			return nil
		})
		if err != nil {
			fatalf("scan browser source %s: %v", scanRoot, err)
		}
	}
	collect(filepath.Join(root, "public"), func(path string) bool {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".js" || ext == ".mjs"
	})
	collect(filepath.Join(root, "dist", "public"), func(path string) bool {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".js" || ext == ".mjs"
	})
	collect(filepath.Join(root, "dist", "static"), func(path string) bool {
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".js" && ext != ".mjs" {
			return false
		}
		frameworkRoot := filepath.Join(root, "dist", "static", "gosx", "assets", "runtime")
		relative, err := filepath.Rel(frameworkRoot, path)
		return err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
	})
	var bytes int64
	for path := range paths {
		bytes += fileSize(path)
		fmt.Printf("  bespoke browser source     %s\n", relativePath(root, path))
	}
	return len(paths), bytes
}

func largestIslandProgram(root string) compressedArtifact {
	var largest compressedArtifact
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		extension := strings.ToLower(filepath.Ext(path))
		if entry.IsDir() || (extension != ".gxi" && extension != ".json") {
			return nil
		}
		artifact := compressedArtifact{
			Path:   path,
			Raw:    fileSize(path),
			Gzip:   compressedFileSize(path, ".gz"),
			Brotli: compressedFileSize(path, ".br"),
		}
		if artifact.Raw > largest.Raw {
			largest = artifact
		}
		return nil
	})
	if err != nil {
		fatalf("scan island programs: %v", err)
	}
	if largest.Path == "" {
		fatalf("no island programs found under %s", root)
	}
	return largest
}

func compressedFileSize(path, suffix string) int64 {
	compressed := path + suffix
	if _, err := os.Stat(compressed); err != nil {
		fatalf("stat precompressed asset %s: %v", compressed, err)
	}
	return fileSize(compressed)
}

func routeClientTransfers(root string, routes []routeSource) []routeTransfer {
	transfers := make([]routeTransfer, 0, len(routes))
	for _, route := range routes {
		seen := make(map[string]bool)
		transfer := routeTransfer{Path: route.Path}
		for _, match := range gosxAssetPattern.FindAllString(string(route.HTML), -1) {
			if seen[match] {
				continue
			}
			seen[match] = true
			assetPath := filepath.Join(root, "dist", filepath.FromSlash(strings.TrimPrefix(match, "/gosx/")))
			transfer.Gzip += compressedFileSize(assetPath, ".gz")
			transfer.Brotli += compressedFileSize(assetPath, ".br")
			transfer.Assets++
		}
		transfers = append(transfers, transfer)
	}
	sort.Slice(transfers, func(i, j int) bool { return transfers[i].Path < transfers[j].Path })
	return transfers
}

func representativeRouteSources(root string, expected map[string][]string) []routeSource {
	var manifest exportManifest
	readJSON(filepath.Join(root, "dist", "export.json"), &manifest)

	sources := make(map[string][]byte, len(manifest.Routes)+len(expected))
	for _, route := range manifest.Routes {
		htmlPath := filepath.Join(root, "dist", "static", filepath.FromSlash(route.File))
		html, err := os.ReadFile(htmlPath)
		if err != nil {
			fatalf("read static route %s: %v", route.Path, err)
		}
		sources[route.Path] = html
	}

	missing := make([]string, 0)
	for path := range expected {
		if _, found := sources[path]; !found {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		for path, html := range fetchReleaseRoutes(root, missing) {
			sources[path] = html
		}
	}

	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	routes := make([]routeSource, 0, len(paths))
	for _, path := range paths {
		routes = append(routes, routeSource{Path: path, HTML: sources[path]})
	}
	return routes
}

func fetchReleaseRoutes(root string, paths []string) map[string][]byte {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatalf("reserve release server port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		fatalf("release reserved port: %v", err)
	}
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	distDir := filepath.Join(root, "dist")
	command := exec.Command(filepath.Join(distDir, "run.sh"))
	command.Dir = distDir
	command.Env = releaseServerEnvironment(distDir, baseURL, port)
	var logs bytes.Buffer
	command.Stdout = &logs
	command.Stderr = &logs
	if err := command.Start(); err != nil {
		fatalf("start release server: %v", err)
	}
	defer func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(15 * time.Second)
	for {
		response, requestErr := client.Get(baseURL + "/api/health")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			fatalf("release server did not become ready: %s", strings.TrimSpace(logs.String()))
		}
		time.Sleep(50 * time.Millisecond)
	}

	sources := make(map[string][]byte, len(paths))
	for _, path := range paths {
		response, err := client.Get(baseURL + path)
		if err != nil {
			fatalf("fetch representative route %s: %v", path, err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			fatalf("read representative route %s: %v / %v", path, readErr, closeErr)
		}
		if response.StatusCode != http.StatusOK {
			fatalf("representative route %s returned %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
		}
		sources[path] = body
	}
	return sources
}

func releaseServerEnvironment(distDir, baseURL string, port int) []string {
	blocked := map[string]bool{
		"DATA_PATH":     true,
		"DEMO_MODE":     true,
		"GOSX_APP_ROOT": true,
		"PORT":          true,
		"PUBLIC_URL":    true,
	}
	environment := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !blocked[name] {
			environment = append(environment, entry)
		}
	}
	return append(environment,
		"DATA_PATH=:memory:",
		"DEMO_MODE=memory",
		"GOSX_APP_ROOT="+distDir,
		"PORT="+strconv.Itoa(port),
		"PUBLIC_URL="+baseURL,
	)
}

func largestRouteGzip(transfers []routeTransfer) int64 {
	var largest int64
	for _, transfer := range transfers {
		if transfer.Gzip > largest {
			largest = transfer.Gzip
		}
	}
	return largest
}

func largestRouteBrotli(transfers []routeTransfer) int64 {
	var largest int64
	for _, transfer := range transfers {
		if transfer.Brotli > largest {
			largest = transfer.Brotli
		}
	}
	return largest
}

func largestStaticHTML(root string) staticHTMLSize {
	var largest staticHTMLSize
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".html") {
			return nil
		}
		raw := fileSize(path)
		compressed := gzipFileSize(path)
		if raw > largest.Raw {
			largest.RawPath = path
			largest.Raw = raw
		}
		if compressed > largest.Gzip {
			largest.GzipPath = path
			largest.Gzip = compressed
		}
		return nil
	})
	if err != nil {
		fatalf("scan static HTML: %v", err)
	}
	if largest.RawPath == "" || largest.GzipPath == "" {
		fatalf("no static HTML found under %s", root)
	}
	return largest
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func directorySize(root string) int64 {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		fatalf("scan %s: %v", root, err)
	}
	return total
}

func checkRouteCapabilities(root string, routes []routeSource, limits budget) bool {
	var manifest exportManifest
	readJSON(filepath.Join(root, "dist", "export.json"), &manifest)
	violations := make([]string, 0)
	seen := make(map[string]bool, len(routes))
	for _, route := range routes {
		seen[route.Path] = true
		expected, checked := limits.ExpectedRouteCapabilities[route.Path]
		if !checked {
			continue
		}
		actual := routeCapabilitySet(route.HTML, route.Path)
		sort.Strings(actual)
		sort.Strings(expected)
		if strings.Join(actual, ",") != strings.Join(expected, ",") {
			violations = append(violations, fmt.Sprintf("%s: got [%s], want [%s]", route.Path, strings.Join(actual, ", "), strings.Join(expected, ", ")))
		}
	}
	for path := range limits.ExpectedRouteCapabilities {
		if !seen[path] {
			violations = append(violations, path+": expected route is missing")
		}
	}
	sort.Strings(violations)
	fmt.Printf("exported routes              %d (minimum %d)\n", len(manifest.Routes), limits.MinimumExportedRoutes)
	if len(manifest.Routes) < limits.MinimumExportedRoutes {
		fmt.Println("  FAIL: too few exported routes")
		return true
	}
	if len(violations) > 0 {
		fmt.Printf("  FAIL: route capability mismatch: %s\n", strings.Join(violations, "; "))
		return true
	}
	fmt.Printf("route capability contracts   %d exact route sets\n", len(limits.ExpectedRouteCapabilities))
	return false
}

func routeCapabilitySet(source []byte, routePath string) []string {
	text := string(source)
	set := make(map[string]bool)
	if strings.Contains(text, "data-gosx-navigation") {
		set["navigation"] = true
	}
	for _, asset := range gosxAssetPattern.FindAllString(text, -1) {
		if strings.Contains(asset, "bootstrap-") {
			set["bootstrap"] = true
		}
		if strings.HasSuffix(asset, ".wasm") {
			set["wasm"] = true
		}
	}
	match := gosxManifestPattern.FindStringSubmatch(text)
	if len(match) == 2 {
		var manifest map[string]json.RawMessage
		if err := json.Unmarshal([]byte(stdhtml.UnescapeString(match[1])), &manifest); err != nil {
			fatalf("decode route runtime manifest %s: %v", routePath, err)
		}
		for _, capability := range []string{"islands", "controllers", "hubs", "engines"} {
			var entries []json.RawMessage
			if raw := manifest[capability]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &entries); err != nil {
					fatalf("decode %s capability for %s: %v", capability, routePath, err)
				}
				if len(entries) > 0 {
					set[capability] = true
				}
			}
		}
	}
	result := make([]string, 0, len(set))
	for capability := range set {
		result = append(result, capability)
	}
	return result
}

func checkMaximum(label string, actual, maximum int64) bool {
	failed := maximum < 0 || actual > maximum
	status := "PASS"
	if failed {
		status = "FAIL"
	}
	fmt.Printf("%-29s %9d / %-9d %s\n", label, actual, maximum, status)
	return failed
}

func checkLimit(label string, actual, maximum int64) bool {
	failed := maximum <= 0 || actual > maximum
	status := "PASS"
	if failed {
		status = "FAIL"
	}
	fmt.Printf("%-29s %9d / %-9d %s\n", label, actual, maximum, status)
	return failed
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(relative)
}

func fatalf(format string, values ...any) {
	fmt.Fprintf(os.Stderr, "sizecheck: "+format+"\n", values...)
	os.Exit(2)
}

type countWriter struct {
	bytes int64
}

func (writer *countWriter) Write(data []byte) (int, error) {
	writer.bytes += int64(len(data))
	return len(data), nil
}
