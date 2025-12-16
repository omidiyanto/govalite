package storage

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	Client *s3.Client
	Bucket string
	Prefix string // Prefix konfigurasi dari env
}

func NewS3Storage(ctx context.Context, bucket, region, accessKey, secretKey, endpoint, prefix string) (*S3Storage, error) {
	insecureTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: insecureTransport}

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if endpoint != "" {
			return aws.Endpoint{URL: endpoint, SigningRegion: region}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.UsePathStyle = true
		}
	})

	return &S3Storage{Client: client, Bucket: bucket, Prefix: prefix}, nil
}

func (s *S3Storage) Name() string { return "S3" }

func (s *S3Storage) Save(ctx context.Context, filename string, data io.Reader) error {
	// FIX: Gunakan 's3.' bukan 's.'
	_, err := s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(filename),
		Body:   data,
	})
	return err
}

func (s *S3Storage) List(ctx context.Context) ([]FileInfo, error) {
	var files []FileInfo

	// FIX: Gunakan 's3.' bukan 's.' (walaupun di kode sebelumnya ini sudah benar sebagai s3.ListObjectsV2Input, tapi pastikan ini konsisten)
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.Bucket),
		Prefix: aws.String(s.Prefix),
	}

	paginator := s3.NewListObjectsV2Paginator(s.Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.LastModified != nil {
				files = append(files, FileInfo{
					Key:          *obj.Key,
					LastModified: *obj.LastModified,
				})
			}
		}
	}
	return files, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	// FIX: Gunakan 's3.' bukan 's.'
	_, err := s.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	return err
}