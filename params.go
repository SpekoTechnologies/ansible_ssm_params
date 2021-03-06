package main

import "fmt"
import (
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"log"
	"strings"
	"errors"
	"crypto/sha256"
)

type ssmClient struct {
	client *ssm.SSM
}

type parameter struct {
	Name string
	Versions []paramHistory

}

type parameters []parameter

type paramHistory struct {
	Value string
	Version string
}

func (ph paramHistory) checksum(name string) string{
	id := fmt.Sprintf("%s.%s@%s",name,ph.Value,ph.Version)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
}

func NewClient(region string) *ssm.SSM {
	session := session.Must(session.NewSession())
	session.Config.WithRegion(region)
	return ssm.New(session)
}

func (s ssmClient) WithPrefix(prefix string) parameters{
	var names parameters
	resp,err := s.ParamList(prefix)
	if err != nil {
		log.Println("Encountered an error listing params")
		return parameters{}
	}
	for _,param := range resp.Parameters {
		names = append(names,parameter{*param.Name,[]paramHistory{}})
	}
	return names
}

func (s ssmClient) ParamList(filter string) (*ssm.DescribeParametersOutput, error) {
	//limit of 50 parameters, unless extra logic is added to paginate
	params := &ssm.DescribeParametersInput{
		MaxResults: aws.Int64(50),
		Filters: []*ssm.ParametersFilter{
			{
				Values: []*string{
					aws.String(filter),
				},
				Key: aws.String("Name"),
			},
		},
	}
	return s.client.DescribeParameters(params)
}

func (p parameters) IncludeHistory(s ssmClient) parameters{
	var params parameters
	for _,param := range p {
		param.history(s)
		params = append(params,param)
	}
	return params
}

func (p *parameter) history(s ssmClient) {//todo, return error
	pi := &ssm.GetParametersInput{
		Names: []*string{&p.Name},
		WithDecryption: aws.Bool(true),
	}
	hpi := &ssm.GetParameterHistoryInput{
		Name: &p.Name,
		WithDecryption: aws.Bool(true),
	}
	resp, err := s.client.GetParameterHistory(hpi)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	r, err := s.client.GetParameters(pi)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	re, err := s.ParamList(p.Name)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	//todo, guard against empty param
	p.Versions = append(p.Versions,paramHistory{Value:*r.Parameters[0].Value,Version:*re.Parameters[0].Description})
	var hist []paramHistory
	var des string
	for _,param := range resp.Parameters {
		if param.Description != nil{
			des = *param.Description

		}
		val := *param.Value
		hist = append(hist,paramHistory{Value:val,Version:des })
	}
	p.Versions = append(p.Versions,hist...)
	return
}

func (p parameters) withVersion(version string) map[string]string{
	paramsDoc := make(map[string]string)
	for _,param := range p {
		ver,err := param.containsVersion(version)
		if err != nil {
			log.Printf("Error: could not find version: %v for param %v",version,param.Name)
			continue
		}
		ParsedName := strings.Split(param.Name,".") //todo, check if envName matches ENV VAR regex
		envName := ParsedName[len(ParsedName) - 1]
		paramsDoc[envName] = ver.Value
	}
	return paramsDoc
}

func (p parameter) containsVersion(version string) (paramHistory,error) {
	for _,v := range p.Versions {
		if v.Version == version {
			return v,nil
		}
	}
	return paramHistory{},errors.New("Could not find version")
}