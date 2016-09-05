package aws

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/convox/rack/api/structs"
)

func (p *AWSProvider) CertificateCreate(pub, key, chain string) (*structs.Certificate, error) {
	end, _ := pem.Decode([]byte(pub))
	pub = string(pem.EncodeToMemory(end))

	c, err := x509.ParseCertificate(end.Bytes)
	if err != nil {
		return nil, err
	}

	req := &iam.UploadServerCertificateInput{
		CertificateBody:       aws.String(pub),
		PrivateKey:            aws.String(key),
		ServerCertificateName: aws.String(fmt.Sprintf("cert-%d", time.Now().Unix())),
	}

	if chain != "" {
		req.CertificateChain = aws.String(chain)
	}

	res, err := p.IAM.UploadServerCertificate(req)

	if err != nil {
		return nil, err
	}

	parts := strings.Split(*res.ServerCertificateMetadata.Arn, "/")
	id := parts[len(parts)-1]

	cert := structs.Certificate{
		Id:         id,
		Domain:     c.Subject.CommonName,
		Expiration: *res.ServerCertificateMetadata.Expiration,
	}

	return &cert, nil
}

func (p *AWSProvider) CertificateDelete(id string) error {
	_, err := p.IAM.DeleteServerCertificate(&iam.DeleteServerCertificateInput{
		ServerCertificateName: aws.String(id),
	})

	return err
}

func (p *AWSProvider) CertificateGenerate(domains []string) (*structs.Certificate, error) {
	if len(domains) < 1 {
		return nil, fmt.Errorf("must specify at least one domain")
	}

	alts := []*string{}

	for _, domain := range domains[1:] {
		alts = append(alts, aws.String(domain))
	}

	req := &acm.RequestCertificateInput{
		DomainName: aws.String(domains[0]),
	}

	if len(alts) > 0 {
		req.SubjectAlternativeNames = alts
	}

	res, err := p.ACM.RequestCertificate(req)

	if err != nil {
		return nil, err
	}

	parts := strings.Split(*res.CertificateArn, "-")
	id := fmt.Sprintf("acm-%s", parts[len(parts)-1])

	cert := structs.Certificate{
		Id:     id,
		Domain: domains[0],
	}

	return &cert, nil
}

func (p *AWSProvider) CertificateList() (structs.Certificates, error) {
	res, err := p.IAM.ListServerCertificates(nil)

	if err != nil {
		return nil, err
	}

	certs := structs.Certificates{}

	for _, cert := range res.ServerCertificateMetadataList {
		res, err := p.IAM.GetServerCertificate(&iam.GetServerCertificateInput{
			ServerCertificateName: cert.ServerCertificateName,
		})
		if err != nil {
			return nil, err
		}

		pem, _ := pem.Decode([]byte(*res.ServerCertificate.CertificateBody))
		if err != nil {
			return nil, err
		}

		c, err := x509.ParseCertificate(pem.Bytes)
		if err != nil {
			return nil, err
		}

		certs = append(certs, structs.Certificate{
			Id:         *cert.ServerCertificateName,
			Domain:     c.Subject.CommonName,
			Expiration: *cert.Expiration,
		})
	}

	c, err := p.certificateListACM()

	if err != nil {
		return nil, err
	}

	certs = append(certs, c...)

	return certs, nil
}

type CfsslCertificateBundle struct {
	Bundle string `json:"bundle"`
}

type CfsslError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e CfsslError) Error() string {
	return e.Message
}

func (p *AWSProvider) certificateListACM() (structs.Certificates, error) {
	certs := structs.Certificates{}

	ares, err := p.ACM.ListCertificates(nil)

	if err != nil {
		return nil, err
	}

	for _, cert := range ares.CertificateSummaryList {
		parts := strings.Split(*cert.CertificateArn, "-")
		id := fmt.Sprintf("acm-%s", parts[len(parts)-1])

		c := structs.Certificate{
			Id:     id,
			Domain: *cert.DomainName,
		}

		res, err := p.ACM.DescribeCertificate(&acm.DescribeCertificateInput{
			CertificateArn: cert.CertificateArn,
		})

		if err != nil {
			return nil, err
		}

		if res.Certificate.NotAfter != nil {
			c.Expiration = *res.Certificate.NotAfter
		}

		certs = append(certs, c)
	}

	return certs, nil
}
