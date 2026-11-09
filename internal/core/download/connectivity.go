package download

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type ConnectivityResult struct {
	Name    string
	Status  int
	Elapsed time.Duration
	Failure string
}

// CheckConnectivity checks HTTP response headers, not bandwidth or complete file
// availability. Only explicit callers initiate it; no credentialed API requests.
func CheckConnectivity(ctx context.Context, client *http.Client, mode SourceMode) ([]ConnectivityResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("连通性检查未配置下载网络")
	}
	if err := (Preferences{Source: mode}).Validate(); err != nil {
		return nil, err
	}
	targets := []struct{ name, address string }{
		{"Mojang 版本目录", "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"},
		{"Minecraft 资源", "https://resources.download.minecraft.net/"},
		{"Minecraft 运行库", "https://libraries.minecraft.net/"},
		{"Modrinth API", "https://api.modrinth.com/v2/tag/game_version"},
		{"CurseForge API（匿名）", "https://api.curseforge.com/v1/games/432"},
		{"Temurin 目录", "https://api.adoptium.net/v3/info/available_releases"},
		{"Java 包分发（GitHub）", "https://github.com/"},
		{"Fabric 目录", "https://meta.fabricmc.net/v2/versions/loader"},
		{"Quilt 目录", "https://meta.quiltmc.org/v3/versions/loader"},
		{"Forge Maven", "https://maven.minecraftforge.net/"},
		{"NeoForge Maven", "https://maven.neoforged.net/"},
	}
	if mode != SourceOfficial {
		targets = append(targets, struct{ name, address string }{"BMCLAPI", "https://bmclapi2.bangbang93.com/"})
	}
	results := make([]ConnectivityResult, len(targets))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range 3 {
		workers.Go(func() {
			for index := range jobs {
				target := targets[index]
				started := time.Now()
				result := ConnectivityResult{Name: target.name}
				task, cancel := context.WithTimeout(ctx, 8*time.Second)
				req, _ := http.NewRequestWithContext(task, http.MethodHead, target.address, nil)
				req.Header.Set("User-Agent", "mcl/0.1")
				host, _ := url.Parse(target.address)
				requestClient := (&Client{HTTP: client}).requestClient(target.address, []string{host.Hostname()}, false)
				requestClient.Timeout = 8 * time.Second
				response, err := requestClient.Do(req)
				result.Elapsed = time.Since(started)
				if err != nil {
					if errors.Is(task.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
						result.Failure = "连接超时"
					} else {
						result.Failure = "连接失败"
					}
				} else {
					result.Status = response.StatusCode
					response.Body.Close()
				}
				cancel()
				results[index] = result
			}
		})
	}
dispatch:
	for i := range targets {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- i:
		}
	}
	close(jobs)
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
