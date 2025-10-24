// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cvm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/hashicorp/packer-plugin-tencentcloud/version"
)

type LogRoundTripper struct{}

func (me *LogRoundTripper) RoundTrip(request *http.Request) (response *http.Response, errRet error) {

	var inBytes, outBytes []byte

	var start = time.Now()

	defer func() { me.log(inBytes, outBytes, errRet, start) }()

	bodyReader, errRet := request.GetBody()
	if errRet != nil {
		return
	}

	var headName = "X-TC-Action"
	var reqClientFormat = version.PluginVersion
	request.Header.Set("X-TC-RequestClient", reqClientFormat.String())
	inBytes = []byte(fmt.Sprintf("%s, request: ", request.Header[headName]))
	requestBody, errRet := ioutil.ReadAll(bodyReader)
	if errRet != nil {
		return
	}

	inBytes = append(inBytes, requestBody...)
	headName = "X-TC-Region"
	appendMessage := []byte(fmt.Sprintf(
		", (host %+v, region:%+v)",
		request.Header["Host"],
		request.Header[headName],
	))

	inBytes = append(inBytes, appendMessage...)
	response, errRet = http.DefaultTransport.RoundTrip(request)
	if errRet != nil {
		return
	}

	outBytes, errRet = ioutil.ReadAll(response.Body)
	if errRet != nil {
		return
	}

	response.Body = ioutil.NopCloser(bytes.NewBuffer(outBytes))
	return
}

func (me *LogRoundTripper) log(in []byte, out []byte, err error, start time.Time) {
	var buf bytes.Buffer
	buf.WriteString("######")
	tag := "[DEBUG]"
	if err != nil {
		tag = "[CRITICAL]"
	}

	buf.WriteString(tag)
	if len(in) > 0 {
		buf.WriteString("tencentcloud-sdk-go: ")
		buf.Write(in)
	}

	if len(out) > 0 {
		buf.WriteString("; response:")
		err := json.Compact(&buf, out)
		if err != nil {
			out := bytes.Replace(out,
				[]byte("\n"),
				[]byte(""),
				-1)
			out = bytes.Replace(out,
				[]byte(" "),
				[]byte(""),
				-1)
			buf.Write(out)
		}
	}

	if err != nil {
		buf.WriteString("; error:")
		buf.WriteString(err.Error())
	}

	costFormat := fmt.Sprintf(",cost %s", time.Since(start).String())
	buf.WriteString(costFormat)

	log.Println(buf.String())
}
