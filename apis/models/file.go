package models

type FileType int32

const (
	FileType_DefaultFileType FileType = 0
	FileType_Image           FileType = 1
	FileType_Audio           FileType = 2
	FileType_Video           FileType = 3
	FileType_File            FileType = 4
	FileType_Log             FileType = 5
)

type QryFileCredReq struct {
	FileType FileType `json:"file_type"`
	Ext      string   `json:"ext"`
}

type OssType int32

const (
	OssType_DefaultOss OssType = 0
	OssType_QiNiu      OssType = 1
	OssType_S3         OssType = 2
	OssType_Minio      OssType = 3
	OssType_Oss        OssType = 4
)

type QryFileCredResp struct {
	OssType OssType `json:"oss_type"`
	// Domain 是预签名通道（S3 / MinIO / OSS）的对象公开读基地址，
	// 客户端用 Domain + pre_sign_resp.obj_key 拼出最终地址；空串表示只能直传预签名 URL。
	Domain        string         `json:"domain,omitempty"`
	QiNiuCredResp *QiNiuCredResp `json:"qiniu_resp"`
	PreSignResp   *PreSignResp   `json:"pre_sign_resp"`
}

type QiNiuCredResp struct {
	Domain string `json:"domain"`
	Token  string `json:"token"`
}

type PreSignResp struct {
	Url         string `json:"url"`
	ObjKey      string `json:"obj_key"`
	Policy      string `json:"policy"`
	SignVersion string `json:"sign_version"`
	Credential  string `json:"credential"`
	Date        string `json:"date"`
	Signature   string `json:"signature"`
}
