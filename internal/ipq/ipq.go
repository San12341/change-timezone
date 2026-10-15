// Package ipq 通过显式代理查询出口 IP 归属地（ipwho.is）。
package ipq

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"change-timezone/internal/config"
)

// Result 是查询结果。
type Result struct {
	IP         string
	Country    string
	TimezoneID string // IANA 时区名，如 America/New_York
}

// QueryError 表示网络/服务层面的失败（对应退出码 3）。
type QueryError struct{ Reason string }

func (e *QueryError) Error() string { return e.Reason }

// DataError 表示返回数据异常（对应退出码 5）。
type DataError struct{ Reason string }

func (e *DataError) Error() string { return e.Reason }

// Querier 是查询接口（便于测试注入）。
type Querier interface {
	Query(rawURL, proxyAddr string, timeout time.Duration) (Result, error)
}

// Query 通过 proxyAddr（可为空，表示直连）查询出口 IP 归属地。
// 查询请求本身必须走代理出口，否则查到的是真实 IP（需求文档 6.3）。
func Query(rawURL, proxyAddr string, timeout time.Duration) (Result, error) {
	tr := &http.Transport{}
	if proxyAddr != "" {
		u, err := url.Parse(proxyAddr)
		if err != nil {
			return Result{}, &QueryError{Reason: "代理地址无效: " + err.Error()}
		}
		tr.Proxy = http.ProxyURL(u)
	}
	client := &http.Client{Transport: tr, Timeout: timeout}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, &QueryError{Reason: "构造请求失败: " + err.Error()}
	}
	req.Header.Set("User-Agent", "change-timezone/"+config.Version)

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, &QueryError{Reason: "请求失败: " + err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, &QueryError{Reason: "读取响应失败: " + err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, &QueryError{Reason: "HTTP 状态码异常: " + strconv.Itoa(resp.StatusCode)}
	}

	var payload struct {
		IP       string `json:"ip"`
		Success  bool   `json:"success"`
		Message  string `json:"message"`
		Country  string `json:"country"`
		Timezone struct {
			ID string `json:"id"`
		} `json:"timezone"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Result{}, &QueryError{Reason: "响应不是有效的 JSON: " + err.Error()}
	}
	if !payload.Success {
		return Result{}, &QueryError{Reason: "服务返回失败: " + payload.Message}
	}
	if !ValidTZID(payload.Timezone.ID) {
		return Result{}, &DataError{Reason: "timezone.id 为空或格式非法: " + strconv.Quote(payload.Timezone.ID)}
	}
	return Result{IP: payload.IP, Country: payload.Country, TimezoneID: payload.Timezone.ID}, nil
}

// ValidTZID 粗校验 IANA 时区名（仅允许字母数字与 /_-+）。
func ValidTZID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '/' || c == '_' || c == '-' || c == '+':
		default:
			return false
		}
	}
	return true
}
