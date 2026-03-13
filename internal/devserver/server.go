package devserver

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/xiewanpeng/claw-identity/internal/publish"
)

const DefaultAddress = "127.0.0.1:8787"

type Result struct {
	RootDir string `json:"root_dir"`
	Address string `json:"address"`
	URL     string `json:"url"`
}

type Server struct {
	Result Result

	server *http.Server
	done   chan error
}

func Start(root, address string) (*Server, error) {
	handler, resolvedRoot, err := NewHandler(root)
	if err != nil {
		return nil, err
	}

	listenAddr := strings.TrimSpace(address)
	if listenAddr == "" {
		listenAddr = DefaultAddress
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %q: %w", listenAddr, err)
	}

	server := &http.Server{Handler: handler}
	result := Result{
		RootDir: resolvedRoot,
		Address: listener.Addr().String(),
		URL:     listenerURL(listener.Addr()),
	}

	running := &Server{
		Result: result,
		server: server,
		done:   make(chan error, 1),
	}

	go func() {
		running.done <- server.Serve(listener)
	}()

	return running, nil
}

func (s *Server) Done() <-chan error {
	return s.done
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func NewHandler(root string) (http.Handler, string, error) {
	resolvedRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return nil, "", fmt.Errorf("resolve serve directory: %w", err)
	}
	info, err := os.Stat(resolvedRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("serve directory %q does not exist; run linkclaw publish first or pass --dir", resolvedRoot)
		}
		return nil, "", fmt.Errorf("stat serve directory: %w", err)
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("serve directory %q is not a directory", resolvedRoot)
	}
	return handler{root: resolvedRoot}, resolvedRoot, nil
}

type handler struct {
	root string
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	absPath, bundlePath, err := h.resolve(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	file, err := os.Open(absPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}

	if mediaType, ok := publish.ContentTypeForBundlePath(bundlePath); ok {
		w.Header().Set("Content-Type", mediaType)
	} else if guessed := mime.TypeByExtension(filepath.Ext(absPath)); guessed != "" {
		w.Header().Set("Content-Type", guessed)
	}

	http.ServeContent(w, r, filepath.Base(absPath), info.ModTime(), file)
}

func (h handler) resolve(requestPath string) (string, string, error) {
	for _, candidate := range requestCandidates(requestPath) {
		absPath := filepath.Join(h.root, filepath.FromSlash(candidate))
		info, err := os.Stat(absPath)
		if err == nil && info.Mode().IsRegular() {
			return absPath, candidate, nil
		}
	}
	return "", "", os.ErrNotExist
}

func requestCandidates(requestPath string) []string {
	cleaned := cleanRequestPath(requestPath)
	trimmed := strings.TrimPrefix(cleaned, "/")
	if trimmed == "" {
		return []string{"index.html"}
	}
	if strings.HasSuffix(cleaned, "/") {
		return []string{path.Join(trimmed, "index.html")}
	}

	candidates := []string{trimmed}
	if !strings.Contains(path.Base(trimmed), ".") {
		candidates = append(candidates, path.Join(trimmed, "index.html"))
	}
	return candidates
}

func cleanRequestPath(requestPath string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(requestPath))
	if strings.HasSuffix(requestPath, "/") && cleaned != "/" {
		return cleaned + "/"
	}
	return cleaned
}

func listenerURL(addr net.Addr) string {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return "http://" + addr.String()
	}
	host := tcpAddr.IP.String()
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, tcpAddr.Port)
}
