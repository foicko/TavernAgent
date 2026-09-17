package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// 请求体大小上限。
//
// 角色卡会带着内嵌头像与世界书一起提交：导入接口接受 32MB 的原图，转换后的
// 卡片 JSON 必须能原样交给建会话接口，否则会出现「预览成功、开启冒险 413」。
// 这两个接口衡量的是同一份载荷，因此共用同一份额度。
const (
	// maxCardUploadBytes 是角色卡文件（PNG / JSON）的上传上限。
	maxCardUploadBytes = 32 << 20
	// maxCardJSONBytes 是转换后卡片 JSON 的允许上限（导入响应与建会话请求共用）。
	maxCardJSONBytes = 8 << 20
	// maxRequestBytes 是其余普通 JSON 接口的默认上限。
	maxRequestBytes = 1 << 20
)

func decodeJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	return decodeLimitedJSON(w, r, target, limit, false)
}

func decodeLimitedJSON(w http.ResponseWriter, r *http.Request, target any, limit int64, strict bool) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("请求体必须只包含一个 JSON 对象")
	}
	return nil
}

func writeBodyError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, 413, "REQUEST_TOO_LARGE", "请求体超过此接口允许的大小，请缩小内容后重试", false, "")
		return
	}
	writeError(w, 400, "BAD_REQUEST", "请求内容无效；请发送一个合法 JSON 对象", false, "")
}
