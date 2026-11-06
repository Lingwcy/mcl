package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"
)

const (
	microsoftOAuth = "https://login.microsoftonline.com/consumers/oauth2/v2.0/"
	microsoftScope = "XboxLive.signin offline_access"
	callbackPath   = "/oauth/callback"
)

func (m *MicrosoftClient) SignIn(ctx context.Context, openBrowser func(string) error) (Grant, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	code, verifier, redirect, err := m.browserCode(ctx, openBrowser)
	if err != nil {
		return Grant{}, err
	}
	token, err := m.token(ctx, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {redirect},
	})
	if err != nil {
		return Grant{}, err
	}
	if token.Refresh == "" {
		return Grant{}, errors.New("微软没有返回刷新凭据，请重新授权")
	}
	return m.exchange(ctx, token)
}

func (m *MicrosoftClient) browserCode(ctx context.Context, openBrowser func(string) error) (code, verifier, redirect string, err error) {
	listener, err := new(net.ListenConfig).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return "", "", "", errors.New("无法打开微软登录的本机回调端口")
	}
	defer listener.Close()
	host := fmt.Sprintf("localhost:%d", listener.Addr().(*net.TCPAddr).Port)
	redirect = "http://" + host + callbackPath
	state, verifier := rand.Text(), rand.Text()+rand.Text()
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"client_id": {m.clientID}, "response_type": {"code"}, "response_mode": {"query"}, "redirect_uri": {redirect},
		"scope": {microsoftScope}, "prompt": {"select_account"}, "state": {state},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"},
	}
	type reply struct{ code, failure string }
	result := make(chan reply, 1)
	serveErr := make(chan error, 1)
	var accepted atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+callbackPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		values := r.URL.Query()
		if r.Method != http.MethodGet || r.Host != host || len(values["state"]) != 1 || subtle.ConstantTimeCompare([]byte(values.Get("state")), []byte(state)) != 1 {
			http.Error(w, "无效的授权回调", http.StatusBadRequest)
			return
		}
		code, failure := values.Get("code"), values.Get("error")
		if (code == "") == (failure == "") || len(code) > 8192 || len(values["code"]) > 1 {
			http.Error(w, "授权回调缺少有效结果", http.StatusBadRequest)
			return
		}
		if !accepted.CompareAndSwap(false, true) {
			http.Error(w, "授权回调已处理", http.StatusConflict)
			return
		}
		result <- reply{code, failure}
		_, _ = fmt.Fprint(w, "已收到授权结果，可以关闭此页并返回 mcl。")
	})
	server := &http.Server{
		Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	go func() { serveErr <- server.Serve(listener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	}()
	if err := openBrowser(microsoftOAuth + "authorize?" + query.Encode()); err != nil {
		return "", "", "", errors.New("无法打开系统浏览器")
	}
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", "", "", errors.New("微软登录超时，请重试")
		}
		return "", "", "", ctx.Err()
	case <-serveErr:
		return "", "", "", errors.New("微软登录回调服务已停止")
	case response := <-result:
		if response.failure != "" {
			return "", "", "", errors.New("微软授权未完成，请在浏览器中重新登录")
		}
		return response.code, verifier, redirect, nil
	}
}
