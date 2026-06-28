package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Config struct {
	SiteName         string      `json:"siteName"`
	AdminPassword   string      `json:"adminPassword"`
	WrapperSecret   string      `json:"wrapperSecret"`
	GlobalAPIKey     string      `json:"globalApiKey"`
	RequestTimeoutMs int         `json:"requestTimeoutMs"`
	Workers          int         `json:"workers"`
	QueueSize        int         `json:"queueSize"`
	RetryTimes       int         `json:"retryTimes"`
	APIs             []APIConfig `json:"apis"`
}

type APIConfig struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Group       string   `json:"group"`
	EndpointURL string   `json:"endpointUrl"`
	Method      string   `json:"method"`
	URLParam    string   `json:"urlParam"`
	KeyParam    string   `json:"keyParam"`
	APIKey      string   `json:"apiKey"`
	SampleURL   string   `json:"sampleUrl"`
	Enabled     bool     `json:"enabled"`
	Order       int      `json:"order"`
	Domains     []string `json:"domains"`
}

type Store struct {
	path string
	mu   sync.RWMutex
	cfg  Config
}

var defaultAPIs = []APIConfig{
	{ID: "youtube", Name: "YouTube解析", Group: "海外视频", EndpointURL: "https://api.nycnm.cn/api/v2/youtube", SampleURL: "https://youtu.be/g0W66BptAdw", Domains: []string{"youtube.com", "youtu.be"}},
	{ID: "huya", Name: "虎牙视频解析", Group: "直播/视频", EndpointURL: "https://api.nycnm.cn/api/v2/huya", SampleURL: "https://www.huya.com/video/play/1102925198.html", Domains: []string{"huya.com"}},
	{ID: "wxsph", Name: "微信视频号解析", Group: "国内短视频", EndpointURL: "https://api.nycnm.cn/api/v2/wxsph", SampleURL: "https://weixin.qq.com/sph/AYznnccv9H", Domains: []string{"weixin.qq.com"}},
	{ID: "qianwen", Name: "千问媒体去水印", Group: "AI创作", EndpointURL: "https://api.nycnm.cn/api/v2/qianwen", Domains: []string{"qianwen", "tongyi", "aliyun.com"}},
	{ID: "doubao", Name: "豆包媒体去水印", Group: "AI创作", EndpointURL: "https://api.nycnm.cn/api/v2/doubao", Domains: []string{"doubao.com"}},
	{ID: "jimengai", Name: "即梦AI去水印", Group: "AI创作", EndpointURL: "https://api.nycnm.cn/api/v2/jimengai", Domains: []string{"jimeng", "jianying.com"}},
	{ID: "tiktok", Name: "TikTok视频解析", Group: "海外视频", EndpointURL: "https://api.nycnm.cn/api/v2/tiktok", SampleURL: "https://www.tiktok.com/@scout2015/video/6718335390845095173", Domains: []string{"tiktok.com"}},
	{ID: "zuiyou", Name: "最右视频解析", Group: "社区视频", EndpointURL: "https://api.nycnm.cn/api/v2/zuiyou", SampleURL: "https://share.xiaochuankeji.cn/hybrid/share/post?pid=409909720&vid=2499989659", Domains: []string{"xiaochuankeji.cn", "izuiyou.com"}},
	{ID: "weibo", Name: "微博短视频解析", Group: "社区视频", EndpointURL: "https://api.nycnm.cn/api/v2/weibo", SampleURL: "https://video.weibo.com/show?fid=1034:5213178888388610", Domains: []string{"weibo.com", "weibo.cn"}},
	{ID: "xhs", Name: "小红书视频图文解析", Group: "图文/视频", EndpointURL: "https://api.nycnm.cn/api/v2/xhs", SampleURL: "http://xhslink.com/o/2e2mgpx7Yk9", Domains: []string{"xhslink.com", "xiaohongshu.com"}},
	{ID: "pipigx", Name: "皮皮搞笑去水印", Group: "社区视频", EndpointURL: "https://api.nycnm.cn/api/v2/pipigx", SampleURL: "https://h5.pipigx.com/pp/post/713972441434", Domains: []string{"pipigx.com"}},
	{ID: "bilibili", Name: "哔哩哔哩去水印", Group: "长视频", EndpointURL: "https://api.nycnm.cn/api/v2/bilibili", SampleURL: "https://www.bilibili.com/video/BV1UwknYnE9w/", Domains: []string{"bilibili.com", "b23.tv"}},
	{ID: "dy", Name: "抖音视频图集解析", Group: "国内短视频", EndpointURL: "https://api.nycnm.cn/api/v2/dy", SampleURL: "https://v.douyin.com/M9d3P5PA7LM", Domains: []string{"douyin.com", "iesdouyin.com"}},
	{ID: "tt", Name: "头条视频解析", Group: "资讯视频", EndpointURL: "https://api.nycnm.cn/api/v2/tt", SampleURL: "https://m.toutiao.com/is/oEe4HwA2dRY/", Domains: []string{"toutiao.com"}},
	{ID: "ks", Name: "快手视频图集解析", Group: "国内短视频", EndpointURL: "https://api.nycnm.cn/api/v2/ks", SampleURL: "https://www.kuaishou.com/f/X-3wBPzKx1Kvi1sE", Domains: []string{"kuaishou.com", "gifshow.com"}},
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, _ := json.Marshal(s.cfg)
	var cfg Config
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func (s *Store) Save(cfg Config) error {
	cfg = sanitizeConfig(cfg)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(s.path, data, 0600)
}

func (s *Store) load() error {
	cfg := defaultConfig()
	if data, err := os.ReadFile(s.path); err == nil {
		var saved Config
		if err := json.Unmarshal(data, &saved); err != nil {
			return err
		}
		cfg = mergeConfig(saved)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		cfg.AdminPassword = v
	}
	if v := os.Getenv("WRAPPER_SECRET"); v != "" {
		cfg.WrapperSecret = v
	}
	if cfg.WrapperSecret == "" {
		cfg.WrapperSecret = randomToken(24)
	}
	cfg = sanitizeConfig(cfg)
	s.cfg = cfg
	if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
		return s.Save(cfg)
	}
	return nil
}

func defaultConfig() Config {
	apis := make([]APIConfig, len(defaultAPIs))
	for i, api := range defaultAPIs {
		api.Method = "GET"
		api.URLParam = "url"
		api.KeyParam = "apikey"
		api.Enabled = true
		api.Order = i + 1
		apis[i] = api
	}
	return Config{
		SiteName:         "NewAPI 视频解析 Wrapper",
		AdminPassword:   env("ADMIN_PASSWORD", "Fyb2530+"),
		WrapperSecret:   os.Getenv("WRAPPER_SECRET"),
		RequestTimeoutMs: 45000,
		Workers:          128,
		QueueSize:        10000,
		RetryTimes:       1,
		APIs:             apis,
	}
}

func mergeConfig(saved Config) Config {
	base := defaultConfig()
	base.SiteName = first(saved.SiteName, base.SiteName)
	base.AdminPassword = first(saved.AdminPassword, base.AdminPassword)
	base.WrapperSecret = first(saved.WrapperSecret, base.WrapperSecret)
	base.GlobalAPIKey = saved.GlobalAPIKey
	if saved.RequestTimeoutMs != 0 {
		base.RequestTimeoutMs = saved.RequestTimeoutMs
	}
	if saved.Workers != 0 {
		base.Workers = saved.Workers
	}
	if saved.QueueSize != 0 {
		base.QueueSize = saved.QueueSize
	}
	base.RetryTimes = saved.RetryTimes
	byID := map[string]APIConfig{}
	for _, api := range saved.APIs {
		byID[api.ID] = api
	}
	for i, api := range base.APIs {
		if savedAPI, ok := byID[api.ID]; ok {
			base.APIs[i] = mergeAPI(api, savedAPI)
			delete(byID, api.ID)
		}
	}
	for _, api := range byID {
		base.APIs = append(base.APIs, sanitizeAPI(api, len(base.APIs)+1))
	}
	return base
}

func mergeAPI(base, saved APIConfig) APIConfig {
	if saved.Name != "" {
		base.Name = saved.Name
	}
	if saved.Group != "" {
		base.Group = saved.Group
	}
	if saved.EndpointURL != "" {
		base.EndpointURL = saved.EndpointURL
	}
	if saved.Method != "" {
		base.Method = saved.Method
	}
	if saved.URLParam != "" {
		base.URLParam = saved.URLParam
	}
	if saved.KeyParam != "" {
		base.KeyParam = saved.KeyParam
	}
	if saved.APIKey != "" {
		base.APIKey = saved.APIKey
	}
	if saved.SampleURL != "" {
		base.SampleURL = saved.SampleURL
	}
	if saved.Order != 0 {
		base.Order = saved.Order
	}
	if len(saved.Domains) > 0 {
		base.Domains = saved.Domains
	}
	base.Enabled = saved.Enabled
	return base
}

func sanitizeConfig(cfg Config) Config {
	cfg.SiteName = first(strings.TrimSpace(cfg.SiteName), "NewAPI 视频解析 Wrapper")
	cfg.AdminPassword = first(strings.TrimSpace(cfg.AdminPassword), "Fyb2530+")
	cfg.WrapperSecret = strings.TrimSpace(cfg.WrapperSecret)
	cfg.GlobalAPIKey = strings.TrimSpace(cfg.GlobalAPIKey)
	cfg.RequestTimeoutMs = clamp(cfg.RequestTimeoutMs, 5000, 180000, 45000)
	cfg.Workers = clamp(cfg.Workers, 1, 512, 128)
	cfg.QueueSize = clamp(cfg.QueueSize, 16, 100000, 10000)
	cfg.RetryTimes = clamp(cfg.RetryTimes, 0, 5, 1)
	for i := range cfg.APIs {
		cfg.APIs[i] = sanitizeAPI(cfg.APIs[i], i+1)
	}
	sort.SliceStable(cfg.APIs, func(i, j int) bool { return cfg.APIs[i].Order < cfg.APIs[j].Order })
	return cfg
}

func sanitizeAPI(api APIConfig, order int) APIConfig {
	api.ID = strings.TrimSpace(api.ID)
	api.Name = first(strings.TrimSpace(api.Name), api.ID)
	api.Group = first(strings.TrimSpace(api.Group), "其他")
	api.EndpointURL = strings.TrimSpace(api.EndpointURL)
	api.Method = strings.ToUpper(strings.TrimSpace(api.Method))
	if api.Method != "POST" {
		api.Method = "GET"
	}
	api.URLParam = first(strings.TrimSpace(api.URLParam), "url")
	api.KeyParam = first(strings.TrimSpace(api.KeyParam), "apikey")
	api.APIKey = strings.TrimSpace(api.APIKey)
	api.SampleURL = strings.TrimSpace(api.SampleURL)
	if api.Order == 0 {
		api.Order = order
	}
	for i := range api.Domains {
		api.Domains[i] = strings.ToLower(strings.TrimSpace(api.Domains[i]))
	}
	return api
}

func enabledAPIs(cfg Config) []APIConfig {
	out := make([]APIConfig, 0, len(cfg.APIs))
	for _, api := range cfg.APIs {
		if api.Enabled {
			out = append(out, api)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
