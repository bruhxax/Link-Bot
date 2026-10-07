package miniapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"image"
	"image/gif"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"link-bot/internal/database"
)

const maxBackgroundSize int64 = 50 << 20

var errBackgroundFormat = errors.New("Используйте изображение PNG, JPG, GIF, WebP, AVIF, BMP, SVG или видео MP4, WebM, MOV, OGV")
var errBackgroundSize = errors.New("Фон должен быть не больше 50 МБ")
var errBackgroundDimensions = errors.New("Слишком большое изображение: максимум 8192 пикселя по стороне и 16 мегапикселей")
var errBackgroundGIF = errors.New("Не удалось прочитать GIF или анимация слишком тяжёлая. Уменьшите размер и число кадров или используйте MP4")

func (h *Handler) handleAdminBackgroundUpload(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.isAdministrator() {
		h.writeError(w, 403, "forbidden", "Access denied")
		return
	}
	if r.Method != http.MethodPost {
		h.writeError(w, 405, "method_not_allowed", "Method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBackgroundSize+(512<<10))
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		h.writeError(w, 400, "background_upload_invalid", "Не удалось прочитать файл. Максимум 50 МБ")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("background")
	if err != nil {
		h.writeError(w, 400, "background_missing", "Выберите файл фона")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBackgroundSize+1))
	if err != nil {
		h.writeError(w, 400, "background_upload_invalid", "Не удалось прочитать файл")
		return
	}
	h.saveBackgroundResponse(w, data)
}

func (h *Handler) handleAdminBackgroundImport(w http.ResponseWriter, r *http.Request, sess *session, _ *database.Customer) {
	if !sess.isAdministrator() {
		h.writeError(w, 403, "forbidden", "Access denied")
		return
	}
	if r.Method != http.MethodPost {
		h.writeError(w, 405, "method_not_allowed", "Method not allowed")
		return
	}
	var input struct {
		URL string `json:"url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		h.writeError(w, 400, "background_url_invalid", "Укажите прямую ссылку на файл")
		return
	}
	data, err := downloadBackground(r.Context(), input.URL)
	if err != nil {
		h.writeError(w, 400, "background_import_failed", "Не удалось загрузить фон: нужна доступная прямая HTTP/HTTPS-ссылка на файл до 50 МБ")
		return
	}
	h.saveBackgroundResponse(w, data)
}

func (h *Handler) saveBackgroundResponse(w http.ResponseWriter, data []byte) {
	fileURL, kind, err := storeUploadedBackground(h.logoUploadDir, data)
	if err != nil {
		status, message := 500, "Не удалось сохранить фон"
		if errors.Is(err, errBackgroundSize) {
			status, message = 413, err.Error()
		}
		if errors.Is(err, errBackgroundFormat) {
			status, message = 415, err.Error()
		}
		if errors.Is(err, errBackgroundDimensions) || errors.Is(err, errBackgroundGIF) {
			status, message = 400, err.Error()
		}
		h.writeError(w, status, "background_upload_failed", message)
		return
	}
	poster := ""
	if kind == "gif" {
		firstFrame, decodeErr := gif.Decode(bytes.NewReader(data))
		if decodeErr != nil {
			h.writeError(w, 415, "background_upload_failed", "Не удалось прочитать GIF")
			return
		}
		var output bytes.Buffer
		if err := png.Encode(&output, firstFrame); err == nil {
			poster, _, _ = storeUploadedBackground(h.logoUploadDir, output.Bytes())
		}
	}
	h.writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]string{"url": fileURL, "type": kind, "poster": poster}})
}

func backgroundFormat(data []byte) (string, string, error) {
	if len(data) == 0 {
		return "", "", errBackgroundFormat
	}
	ext, kind := "", "image"
	switch http.DetectContentType(data) {
	case "image/png":
		ext = "png"
	case "image/jpeg":
		ext = "jpg"
	case "image/gif":
		ext, kind = "gif", "gif"
	case "image/webp":
		ext = "webp"
	case "image/bmp":
		ext = "bmp"
	case "video/mp4":
		ext, kind = "mp4", "video"
	case "video/webm":
		ext, kind = "webm", "video"
	default:
		if len(data) >= 16 && string(data[4:8]) == "ftyp" {
			switch string(data[8:12]) {
			case "avif", "avis":
				ext = "avif"
			case "qt  ":
				ext, kind = "mov", "video"
			case "isom", "iso2", "iso5", "iso6", "mp41", "mp42", "M4V ", "MSNV", "dash":
				ext, kind = "mp4", "video"
			}
		} else if len(data) >= 4 && string(data[:4]) == "OggS" && bytes.Contains(data[:min(len(data), 4096)], []byte("theora")) {
			ext, kind = "ogv", "video"
		} else {
			decoder := xml.NewDecoder(bytes.NewReader(data))
			for count := 0; count < 32; count++ {
				token, err := decoder.Token()
				if err != nil {
					break
				}
				if start, ok := token.(xml.StartElement); ok {
					if start.Name.Local == "svg" && (start.Name.Space == "" || start.Name.Space == "http://www.w3.org/2000/svg") {
						ext = "svg"
					}
					break
				}
			}
		}
	}
	if ext == "" {
		return "", "", errBackgroundFormat
	}
	if ext == "png" || ext == "jpg" || ext == "gif" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return "", "", errBackgroundFormat
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 16_777_216 {
			return "", "", errBackgroundDimensions
		}
		if ext == "gif" && !validBackgroundGIF(data) {
			return "", "", errBackgroundGIF
		}
	}
	return ext, kind, nil
}

// Check block boundaries before the browser decoder sees a GIF. This also
// bounds frame allocation/work without decompressing the whole animation.
func validBackgroundGIF(data []byte) bool {
	if len(data) < 13 {
		return false
	}
	if string(data[:6]) != "GIF87a" && string(data[:6]) != "GIF89a" {
		return false
	}
	width, height := int(binary.LittleEndian.Uint16(data[6:8])), int(binary.LittleEndian.Uint16(data[8:10]))
	if width*height > 4_194_304 {
		return false
	}
	pos := 13
	if data[10]&128 != 0 {
		pos += 3 * (1 << ((data[10] & 7) + 1))
	}
	skipBlocks := func() bool {
		for pos < len(data) {
			size := int(data[pos])
			pos++
			if size == 0 {
				return true
			}
			pos += size
			if pos > len(data) {
				return false
			}
		}
		return false
	}
	frames, pixels := 0, 0
	for pos < len(data) {
		kind := data[pos]
		pos++
		switch kind {
		case 0x3b:
			return frames > 0
		case 0x21:
			if pos >= len(data) {
				return false
			}
			label := data[pos]
			pos++
			// The client decoder understands these four extension types. GCE
			// has a fixed payload; accepting another length desynchronizes it.
			switch label {
			case 0xf9:
				if pos+6 > len(data) || data[pos] != 4 || data[pos+5] != 0 {
					return false
				}
			case 0xff, 0xfe, 0x01:
			default:
				return false
			}
			if !skipBlocks() {
				return false
			}
		case 0x2c:
			if pos+9 > len(data) {
				return false
			}
			left, top := int(binary.LittleEndian.Uint16(data[pos:pos+2])), int(binary.LittleEndian.Uint16(data[pos+2:pos+4]))
			w, h := int(binary.LittleEndian.Uint16(data[pos+4:pos+6])), int(binary.LittleEndian.Uint16(data[pos+6:pos+8]))
			packed := data[pos+8]
			pos += 9
			frames++
			pixels += w * h
			if w == 0 || h == 0 || left+w > width || top+h > height || frames > 1000 || pixels > 150_000_000 {
				return false
			}
			if packed&128 != 0 {
				pos += 3 * (1 << ((packed & 7) + 1))
			}
			if pos >= len(data) || data[pos] < 2 || data[pos] > 8 {
				return false
			}
			pos++
			if !skipBlocks() {
				return false
			}
		default:
			return false
		}
	}
	return false
}

func storeUploadedBackground(dir string, data []byte) (string, string, error) {
	if int64(len(data)) > maxBackgroundSize {
		return "", "", errBackgroundSize
	}
	ext, kind, err := backgroundFormat(data)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(dir) == "" {
		return "", "", errors.New("background storage unavailable")
	}
	if err = os.MkdirAll(dir, 0750); err != nil {
		return "", "", err
	}
	hash := sha256.Sum256(data)
	name := "background-" + hex.EncodeToString(hash[:8]) + "." + ext
	final := filepath.Join(dir, name)
	if info, err := os.Stat(final); err == nil && info.Mode().IsRegular() {
		return "/mini-app/uploads/" + name, kind, nil
	}
	tmp, err := os.CreateTemp(dir, ".background-upload-*")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0644); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), final)
		if err != nil {
			if info, statErr := os.Stat(final); statErr == nil && info.Mode().IsRegular() {
				err = nil
			}
		}
	}
	if err != nil {
		return "", "", err
	}
	return "/mini-app/uploads/" + name, kind, nil
}

// Resolve and validate every destination at dial time, including redirects.
// Dialing the checked IP prevents DNS rebinding; TLS still validates the URL host.
func backgroundPublicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "64:ff9b::/96", "64:ff9b:1::/48", "2001:db8::/32", "2001::/32", "2002::/16"} {
		_, network, _ := net.ParseCIDR(block)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

func validateBackgroundURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || (u.Port() != "" && u.Port() != "80" && u.Port() != "443") {
		return nil, errors.New("invalid public media URL")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !backgroundPublicIP(ip) {
		return nil, errors.New("private media URL")
	}
	return u, nil
}

func downloadBackground(ctx context.Context, raw string) ([]byte, error) {
	u, err := validateBackgroundURL(raw)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{DisableKeepAlives: true, ResponseHeaderTimeout: 15 * time.Second, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("empty DNS response")
		}
		for _, ip := range ips {
			if !backgroundPublicIP(ip.IP) {
				return nil, errors.New("private destination")
			}
		}
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		var lastErr error
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		_, err := validateBackgroundURL(req.URL.String())
		return err
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("media server did not return a file")
	}
	if response.ContentLength > maxBackgroundSize {
		return nil, errBackgroundSize
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBackgroundSize+1))
	if err == nil && int64(len(data)) > maxBackgroundSize {
		err = errBackgroundSize
	}
	return data, err
}
