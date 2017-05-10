/*
 *
 * k6 - a next-generation load testing tool
 * Copyright (C) 2017 Load Impact
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

const (
	TIMEOUT = 10 * time.Second
)

// Client handles communication with Load Impact cloud API.
type Client struct {
	client  *http.Client
	token   string
	baseURL string
	version string
}

func NewClient(token, host, version string) *Client {
	client := &http.Client{
		Timeout: TIMEOUT,
	}

	hostEnv := os.Getenv("K6CLOUD_HOST")
	if hostEnv != "" {
		host = hostEnv
	}
	if host == "" {
		host = "https://ingest.loadimpact.com"
	}

	baseURL := fmt.Sprintf("%s/v1", host)

	c := &Client{
		client:  client,
		token:   token,
		baseURL: baseURL,
		version: version,
	}
	return c
}

func (c *Client) NewRequest(method, url string, data interface{}) (*http.Request, error) {
	var buf io.Reader

	if data != nil {
		b, err := json.Marshal(&data)
		if err != nil {
			return nil, err
		}

		buf = bytes.NewBuffer(b)
	}

	return http.NewRequest(method, url, buf)
}

func (c *Client) Do(req *http.Request, v interface{}) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.token))
	req.Header.Set("User-Agent", "k6cloud/"+c.version)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Errorln(err)
		}
	}()

	if err = checkResponse(resp); err != nil {
		return err
	}

	if v != nil {
		if err = json.NewDecoder(resp.Body).Decode(v); err == io.EOF {
			err = nil // Ignore EOF from empty body
		}
	}

	return err
}

func checkResponse(r *http.Response) error {
	if c := r.StatusCode; c >= 200 && c <= 299 {
		return nil
	}

	if r.StatusCode == 401 {
		return ErrNotAuthenticated
	} else if r.StatusCode == 403 {
		return ErrNotAuthorized
	}

	// Struct of errors set back from API
	errorStruct := &struct {
		ErrorData struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(errorStruct); err != nil {
		return errors.Wrap(err, "Non-standard API error response")
	}

	errorResponse := &ErrorResponse{
		Response: r,
		Message:  errorStruct.ErrorData.Message,
		Code:     errorStruct.ErrorData.Code,
	}

	return errorResponse
}
