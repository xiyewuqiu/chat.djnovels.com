package fileengines

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/juggleim/jugglechat-server/commons/tools"
)

// S3Config 是 S3 兼容对象存储的通道配置：AWS S3、Cloudflare R2，以及任何 S3 网关。
type S3Config struct {
	AccessKey string `json:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	// Domain 是对象的公开读基地址（自定义域名 / CDN）。客户端用 Domain + ObjKey 拼最终地址，
	// 因此它必须指向同一个 bucket 的根。留空表示不提供公开读，客户端只能用 15 分钟有效的预签名 URL。
	Domain string `json:"domain,omitempty"`
	// PublicRead 打开后会在预签名里附带 x-amz-acl:public-read（AWS S3 的对象级公开读）。
	// 该头会被计入签名，客户端必须原样回传，否则拿到一句与真实原因无关的
	// SignatureDoesNotMatch。R2 / MinIO 没有对象 ACL，留空即可（bucket 级公开读更省事）。
	PublicRead bool `json:"public_read,omitempty"`
}

type S3Storage struct {
	accessKey        string
	secretKey        string
	endpoint         string
	region           string
	bucket           string
	domain           string
	publicRead       bool
	disableSSL       bool
	s3forcePathStyle bool
}

type Option func(*S3Storage)

func WithAccessKey(accessKey string) Option {
	return func(o *S3Storage) {
		o.accessKey = accessKey
	}
}
func WithSecretKey(secretKey string) Option {
	return func(o *S3Storage) {
		o.secretKey = secretKey
	}
}
func WithEndpoint(endpoint string) Option {
	return func(o *S3Storage) {
		o.endpoint = endpoint
	}
}
func WithRegion(region string) Option {
	return func(o *S3Storage) {
		o.region = region
	}
}
func WithBucket(bucket string) Option {
	return func(o *S3Storage) {
		o.bucket = bucket
	}
}
func WithDomain(domain string) Option {
	return func(o *S3Storage) {
		o.domain = domain
	}
}
func WithPublicRead(publicRead bool) Option {
	return func(o *S3Storage) {
		o.publicRead = publicRead
	}
}
func WithConf(conf S3Config) Option {
	return func(o *S3Storage) {
		o.accessKey = conf.AccessKey
		o.secretKey = conf.SecretKey
		o.endpoint = conf.Endpoint
		o.region = conf.Region
		o.bucket = conf.Bucket
		o.domain = conf.Domain
		o.publicRead = conf.PublicRead
	}
}

func NewS3Storage(options ...Option) *S3Storage {
	s := &S3Storage{}
	for _, option := range options {
		option(s)
	}

	return s
}

// Domain 返回公开读基地址；空串表示未配置公开读。
func (s *S3Storage) Domain() string {
	return s.domain
}

func (s *S3Storage) putPreSignURL(fileType string, dir string) (url string, objKey string, err error) {
	sess, err := session.NewSession(&aws.Config{
		Credentials:      credentials.NewStaticCredentials(s.accessKey, s.secretKey, ""),
		Endpoint:         aws.String(s.endpoint),
		Region:           aws.String(s.region),
		DisableSSL:       aws.Bool(s.disableSSL),
		S3ForcePathStyle: aws.Bool(s.s3forcePathStyle), //virtual-host style方式
	})
	if err != nil {
		err = fmt.Errorf("error creating S3 session: %v", err)
		return "", "", err
	}
	svc := s3.New(sess)

	objectName := tools.GenerateUUIDShort22() + "." + fileType
	objectName = filepath.Join(dir, objectName)
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectName),
	}
	if s.publicRead {
		input.ACL = aws.String("public-read")
	}
	req, _ := svc.PutObjectRequest(input)
	url, err = req.Presign(15 * time.Minute)

	return url, objectName, err
}

func (s *S3Storage) PreSignedURL(fileType string, dir string) (url string, err error) {
	url, _, err = s.PreSignedURLWithKey(fileType, dir)
	return
}

// PreSignedURLWithKey 在预签名 URL 之外额外返回对象 key：
// 调用方用 Domain + key 拼出可直读的地址，后续做对象删除 / 迁移也不必再解析 URL。
func (s *S3Storage) PreSignedURLWithKey(fileType string, dir string) (url string, objKey string, err error) {
	return s.putPreSignURL(fileType, dir)
}
