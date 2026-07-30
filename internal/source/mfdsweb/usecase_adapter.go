package mfdsweb

import (
	"context"
	"errors"

	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

type UsecaseAdapter struct {
	client *Client
}

func NewUsecaseAdapter(client *Client) *UsecaseAdapter {
	return &UsecaseAdapter{client: client}
}

func (a *UsecaseAdapter) FetchList(ctx context.Context, request weblist.ListRequest) (weblist.Fetch, error) {
	sourceRequest := toSourceRequest(request)
	artifact, err := a.client.FetchList(ctx, sourceRequest)
	return weblist.Fetch{
		Method: artifact.Method, URL: artifact.URL, QueryJSON: artifact.QueryJSON,
		RequestHeadersJSON: artifact.RequestHeadersJSON, RequestKeySHA256: artifact.RequestKeySHA256,
		StartedAt: artifact.StartedAt, FinishedAt: artifact.FinishedAt, Duration: artifact.Duration,
		HTTPStatus: artifact.HTTPStatus, ResponseHeadersJSON: artifact.ResponseHeadersJSON,
		ContentType: artifact.ContentType, Body: artifact.Body, BodyGZIP: artifact.BodyGZIP,
		BodySize: artifact.BodySize, BodySHA256: artifact.BodySHA256,
		BodyCaptured: artifact.BodyGZIP != nil,
	}, err
}

func (a *UsecaseAdapter) ParseList(body []byte, request weblist.ListRequest) (weblist.Page, error) {
	page, err := ParseList(body, toSourceRequest(request))
	if err != nil {
		return weblist.Page{}, err
	}
	rows := make([]weblist.Row, len(page.Rows))
	for index, row := range page.Rows {
		rows[index] = weblist.Row{
			RowNo: row.RowNo, RCNO: row.RCNO, ProductDivisionName: row.ProductDivisionName,
			ImporterName: row.ImporterName, ProductNameKO: row.ProductNameKO,
			ProductNameEN: row.ProductNameEN, ItemName: row.ItemName,
			OverseasEstablishmentName: row.OverseasEstablishmentName,
			ProcessedDateRaw:          row.ProcessedDateRaw, ProcessedDate: row.ProcessedDate,
			ExpiryText: row.ExpiryText, ManufactureCountryName: row.ManufactureCountryName,
			ExportCountryName: row.ExportCountryName, DetailHref: row.DetailHref,
			CanonicalValuesJSON: row.CanonicalValuesJSON, RawRowHTML: row.RawRowHTML,
			RawRowSHA256: row.RawRowSHA256, SemanticSHA256: row.SemanticSHA256,
		}
	}
	return weblist.Page{
		Total: page.Total, Page: page.Page, Limit: page.Limit, TotalPages: page.TotalPages,
		Rows: rows, Warnings: append([]string(nil), page.Warnings...),
	}, nil
}

func (a *UsecaseAdapter) ErrorKind(err error) string {
	if err == nil {
		return ""
	}
	var adapterError *AdapterError
	if errors.As(err, &adapterError) {
		return string(adapterError.Kind)
	}
	return ""
}

func toSourceRequest(request weblist.ListRequest) ListRequest {
	return ListRequest{
		Item:        Item{Name: request.ItemName, Code: request.ItemCode},
		ProcessDate: request.ProcessDate, Page: request.Page, Limit: request.Limit,
		TotalSnapshot: request.TotalSnapshot,
	}
}

var _ weblist.ListSource = (*UsecaseAdapter)(nil)
