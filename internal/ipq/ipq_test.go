package ipq

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQuerySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ip":"8.8.4.4","success":true,"country":"United States","timezone":{"id":"America/Los_Angeles","abbr":"PDT"}}`)
	}))
	defer srv.Close()
	res, err := Query(srv.URL, "", 5*time.Second)
	if err != nil {
		t.Fatalf("不应失败: %v", err)
	}
	if res.IP != "8.8.4.4" || res.Country != "United States" || res.TimezoneID != "America/Los_Angeles" {
		t.Fatalf("结果不正确: %+v", res)
	}
}

func TestQueryThroughProxy(t *testing.T) {
	// 目标服务器：不应被直接访问
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
		io.WriteString(w, `{"ip":"wrong","success":true,"timezone":{"id":"Europe/Berlin"}}`)
	}))
	defer target.Close()

	// 假代理：直接以代理身份返回固定响应
	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ip":"1.2.3.4","success":true,"country":"Testland","timezone":{"id":"Europe/Berlin"}}`)
	}))
	defer proxy.Close()

	res, err := Query(target.URL, proxy.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("不应失败: %v", err)
	}
	if proxyHits != 1 {
		t.Fatalf("请求应经过代理（命中 %d 次）", proxyHits)
	}
	if targetHits != 0 {
		t.Fatalf("不应直连目标服务器（命中 %d 次）", targetHits)
	}
	if res.IP != "1.2.3.4" || res.TimezoneID != "Europe/Berlin" {
		t.Fatalf("结果不正确: %+v", res)
	}
}

func TestQueryFailures(t *testing.T) {
	t.Run("success为false", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"ip":"1.2.3.4","success":false,"message":"quota exceeded"}`)
		}))
		defer srv.Close()
		_, err := Query(srv.URL, "", 5*time.Second)
		if _, ok := err.(*QueryError); !ok {
			t.Fatalf("应为 QueryError: %v", err)
		}
	})
	t.Run("非法JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `<html>bad gateway</html>`)
		}))
		defer srv.Close()
		_, err := Query(srv.URL, "", 5*time.Second)
		if _, ok := err.(*QueryError); !ok {
			t.Fatalf("应为 QueryError: %v", err)
		}
	})
	t.Run("HTTP异常状态", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		_, err := Query(srv.URL, "", 5*time.Second)
		if _, ok := err.(*QueryError); !ok {
			t.Fatalf("应为 QueryError: %v", err)
		}
	})
	t.Run("超时", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			io.WriteString(w, `{}`)
		}))
		defer srv.Close()
		_, err := Query(srv.URL, "", 30*time.Millisecond)
		if _, ok := err.(*QueryError); !ok {
			t.Fatalf("应为 QueryError: %v", err)
		}
	})
	t.Run("时区字段为空", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"ip":"1.2.3.4","success":true,"timezone":{"id":""}}`)
		}))
		defer srv.Close()
		_, err := Query(srv.URL, "", 5*time.Second)
		if _, ok := err.(*DataError); !ok {
			t.Fatalf("应为 DataError: %v", err)
		}
	})
	t.Run("时区字段非法", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"ip":"1.2.3.4","success":true,"timezone":{"id":"Bad Zone Name"}}`)
		}))
		defer srv.Close()
		_, err := Query(srv.URL, "", 5*time.Second)
		if _, ok := err.(*DataError); !ok {
			t.Fatalf("应为 DataError: %v", err)
		}
	})
}

func TestValidTZID(t *testing.T) {
	valid := []string{"America/New_York", "Etc/GMT+5", "UTC", "America/Argentina/Buenos_Aires"}
	for _, s := range valid {
		if !ValidTZID(s) {
			t.Errorf("%q 应合法", s)
		}
	}
	invalid := []string{"", "Bad Zone", "America/New\nYork", strings.Repeat("a", 65)}
	for _, s := range invalid {
		if ValidTZID(s) {
			t.Errorf("%q 应非法", s)
		}
	}
}
