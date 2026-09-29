package widgets

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // GIF (first frame)
	_ "image/jpeg" // JPEG
	_ "image/png"  // PNG
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/bmp" // BMP
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff" // TIFF
	_ "golang.org/x/image/webp" // WebP

	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/resources"
)

// ImageSource says where an Image comes from. Implement it for your own
// sources (a database, a zip, a generated picture…).
type ImageSource interface {
	// CacheKey identifies the image: sources with equal keys share one
	// decoded image in the cache.
	CacheKey() string
	// Load returns the encoded image (PNG, JPEG, GIF, WebP, BMP, TIFF).
	// It runs on a background goroutine; ctx is cancelled when nothing
	// needs the image anymore.
	Load(ctx context.Context) ([]byte, error)
}

// FileImage loads an image from the file system.
type FileImage struct {
	Path string
}

func (f FileImage) CacheKey() string {
	p, err := filepath.Abs(f.Path)
	if err != nil {
		p = f.Path
	}
	return "file:" + p
}

func (f FileImage) Load(context.Context) ([]byte, error) { return os.ReadFile(f.Path) }

// AssetImage loads an image from the app's resources (see package
// resources), e.g. AssetImage{Name: "images/logo.png"}.
type AssetImage struct {
	Name string
}

func (a AssetImage) CacheKey() string { return "asset:" + a.Name }

func (a AssetImage) Load(context.Context) ([]byte, error) { return resources.ReadFile(a.Name) }

// MemoryImage decodes an image already in memory. Key names it for the
// cache; without one the bytes are hashed.
type MemoryImage struct {
	Data []byte
	Key  string
}

func (m MemoryImage) CacheKey() string {
	if m.Key != "" {
		return "mem:" + m.Key
	}
	sum := sha256.Sum256(m.Data)
	return "mem#" + hex.EncodeToString(sum[:12])
}

func (m MemoryImage) Load(context.Context) ([]byte, error) { return m.Data, nil }

// NetworkImage downloads an image over HTTP(S).
//
//	NetworkImage{URL: "https://example.com/a.png"}
//	NetworkImage{URL: api + "/avatar", Headers: map[string]string{"Authorization": "Bearer " + tok}}
//	NetworkImage{URL: api + "/render", Method: "POST", Body: payload,
//	    Headers: map[string]string{"Content-Type": "application/json"}}
//
// Only https:// is allowed unless AllowHTTP is set, so a typo or a
// redirect can't send credentials in clear text.
type NetworkImage struct {
	URL string
	// Method defaults to GET, or POST when Body is set.
	Method string
	// Headers are added to the request (e.g. Authorization, Accept).
	Headers map[string]string
	// Body is sent as the request body.
	Body []byte
	// AllowHTTP permits plain http:// URLs (and redirects to them).
	AllowHTTP bool
	// Timeout for the whole request; default 30s.
	Timeout time.Duration
	// MaxBytes caps the download; default 64 MiB.
	MaxBytes int64
	// Client sends the request (default http.DefaultClient's transport);
	// set it for proxies, custom TLS or cookies.
	Client *http.Client
	// Key overrides the cache key (by default: method, URL, headers, body).
	Key string
}

func (n NetworkImage) method() string {
	if n.Method != "" {
		return strings.ToUpper(n.Method)
	}
	if n.Body != nil {
		return http.MethodPost
	}
	return http.MethodGet
}

func (n NetworkImage) CacheKey() string {
	if n.Key != "" {
		return "net:" + n.Key
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s %s\n", n.method(), n.URL)
	keys := make([]string, 0, len(n.Headers))
	for k := range n.Headers {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "%s: %s\n", strings.ToLower(k), n.Headers[k])
	}
	h.Write(n.Body)
	return "net#" + hex.EncodeToString(h.Sum(nil)[:16])
}

// ErrInsecureURL is returned for http:// URLs when AllowHTTP isn't set.
var ErrInsecureURL = errors.New("image: plain http:// is not allowed (set NetworkImage.AllowHTTP)")

func (n NetworkImage) checkURL(u *url.URL) error {
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if n.AllowHTTP {
			return nil
		}
		return ErrInsecureURL
	}
	return fmt.Errorf("image: unsupported URL scheme %q", u.Scheme)
}

func (n NetworkImage) Load(ctx context.Context) ([]byte, error) {
	u, err := url.Parse(n.URL)
	if err != nil {
		return nil, err
	}
	if err := n.checkURL(u); err != nil {
		return nil, err
	}
	timeout := n.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var body io.Reader
	if n.Body != nil {
		body = bytes.NewReader(n.Body)
	}
	req, err := http.NewRequestWithContext(ctx, n.method(), n.URL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range n.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "image/*")
	}
	client := http.Client{}
	if n.Client != nil {
		client = *n.Client
	}
	// Re-check every redirect hop against the scheme policy.
	prev := client.CheckRedirect
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if err := n.checkURL(r.URL); err != nil {
			return err
		}
		if prev != nil {
			return prev(r, via)
		}
		if len(via) >= 10 {
			return errors.New("image: too many redirects")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("image: %s %s: %s", n.method(), n.URL, resp.Status)
	}
	limit := n.MaxBytes
	if limit <= 0 {
		limit = 64 << 20
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("image: %s is larger than %d bytes", n.URL, limit)
	}
	return data, nil
}

// ---------------------------------------------------------------------------
// Decoding

// MaxImageSize caps decoded images (largest side, in pixels); bigger ones
// are scaled down on decode to save memory. 0 = no limit.
var MaxImageSize = 4096

// DecodeImage decodes PNG, JPEG, GIF (first frame), WebP, BMP or TIFF data
// into a drawable image, scaled down to MaxImageSize.
func DecodeImage(data []byte) (*render.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: decode: %w", err)
	}
	b := img.Bounds()
	if lim := MaxImageSize; lim > 0 && (b.Dx() > lim || b.Dy() > lim) {
		s := float64(lim) / float64(max(b.Dx(), b.Dy()))
		small := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*s)), max(1, int(float64(b.Dy())*s))))
		xdraw.CatmullRom.Scale(small, small.Bounds(), img, b, xdraw.Src, nil)
		img = small
	}
	return render.NewImage(img), nil
}

// ---------------------------------------------------------------------------
// Cache

// ImageCache keeps decoded images so the same source is loaded and decoded
// once, and shares in-flight loads between widgets. It evicts the least
// recently used images above MaxBytes (decoded size). Failed loads aren't
// cached.
type ImageCache struct {
	mu       sync.Mutex
	maxBytes int
	bytes    int
	lru      *list.List // of *cacheEntry, most recent at the front
	entries  map[string]*list.Element
	loading  map[string]*pendingLoad
}

type cacheEntry struct {
	key string
	img *render.Image
}

type pendingLoad struct {
	cancel  context.CancelFunc
	waiters map[int]func(*render.Image, error)
}

// NewImageCache makes a cache holding up to maxBytes of decoded pixels.
func NewImageCache(maxBytes int) *ImageCache {
	return &ImageCache{maxBytes: maxBytes, lru: list.New(),
		entries: map[string]*list.Element{}, loading: map[string]*pendingLoad{}}
}

// Images is the cache Image widgets use by default (256 MiB).
var Images = NewImageCache(256 << 20)

// Get returns the cached image for src, if loaded.
func (c *ImageCache) Get(src ImageSource) (*render.Image, bool) {
	return c.get(src.CacheKey())
}

func (c *ImageCache) get(key string) (*render.Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.lru.MoveToFront(el)
		return el.Value.(*cacheEntry).img, true
	}
	return nil, false
}

// Put stores an image under src's key.
func (c *ImageCache) Put(src ImageSource, img *render.Image) { c.put(src.CacheKey(), img) }

func (c *ImageCache) put(key string, img *render.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.bytes -= el.Value.(*cacheEntry).img.Bytes()
		c.lru.Remove(el)
	}
	c.entries[key] = c.lru.PushFront(&cacheEntry{key, img})
	c.bytes += img.Bytes()
	for c.bytes > c.maxBytes && c.lru.Len() > 1 {
		el := c.lru.Back()
		e := el.Value.(*cacheEntry)
		c.lru.Remove(el)
		delete(c.entries, e.key)
		c.bytes -= e.img.Bytes()
	}
}

// Evict forgets src (e.g. after the file or URL content changed); Image
// widgets showing it reload the next time they're built with a new source.
func (c *ImageCache) Evict(src ImageSource) {
	key := src.CacheKey()
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.bytes -= el.Value.(*cacheEntry).img.Bytes()
		c.lru.Remove(el)
		delete(c.entries, key)
	}
}

// Clear empties the cache.
func (c *ImageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lru.Init()
	clear(c.entries)
	c.bytes = 0
}

// Stats reports the number of cached images and their decoded size.
func (c *ImageCache) Stats() (count, bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len(), c.bytes
}

var waiterIDs struct {
	sync.Mutex
	n int
}

// Load gets src from the cache or loads it in the background; done runs
// (on the loading goroutine) with the result. Concurrent loads of the same
// key are shared. The returned func cancels interest; the load itself is
// cancelled when nobody waits for it anymore.
func (c *ImageCache) Load(src ImageSource, done func(*render.Image, error)) (cancel func()) {
	key := src.CacheKey()
	if img, ok := c.get(key); ok {
		done(img, nil)
		return func() {}
	}
	waiterIDs.Lock()
	waiterIDs.n++
	id := waiterIDs.n
	waiterIDs.Unlock()

	c.mu.Lock()
	p, running := c.loading[key]
	if !running {
		ctx, cancelLoad := context.WithCancel(context.Background())
		p = &pendingLoad{cancel: cancelLoad, waiters: map[int]func(*render.Image, error){}}
		c.loading[key] = p
		go c.run(ctx, key, src, p)
	}
	p.waiters[id] = done
	c.mu.Unlock()

	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(p.waiters, id)
		if len(p.waiters) == 0 && c.loading[key] == p {
			p.cancel()
		}
	}
}

func (c *ImageCache) run(ctx context.Context, key string, src ImageSource, p *pendingLoad) {
	var img *render.Image
	data, err := src.Load(ctx)
	if err == nil {
		img, err = DecodeImage(data)
	}
	if err == nil {
		c.put(key, img)
	}
	c.mu.Lock()
	if c.loading[key] == p {
		delete(c.loading, key)
	}
	waiters := make([]func(*render.Image, error), 0, len(p.waiters))
	for _, w := range p.waiters {
		waiters = append(waiters, w)
	}
	c.mu.Unlock()
	p.cancel()
	for _, w := range waiters {
		w(img, err)
	}
}
