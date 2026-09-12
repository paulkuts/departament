package models

type Paginated struct {
	Limit  int64 `form:"limit" validate:"gte=10,lte=100"`
	Offset int64 `form:"offset" validate:"gte=0"`
}

func (p *Paginated) Validate() {
	if p.Limit <= 0 {
		p.Limit = 1
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
}

type PaginatedMetadata struct {
	Page        int64 `json:"page"`
	Total       int64 `json:"total"`
	TotalPages  int64 `json:"total_pages"`
	HasNextPage bool  `json:"has_next_page"`
	HasPrevPage bool  `json:"has_prev_page"`
}

func MakePaginatedMetadata(limit, offset, total int64) PaginatedMetadata {
	if limit <= 0 {
		limit = 1
	}

	totalPages := int64(0)
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	return PaginatedMetadata{
		Page:        offset/limit + 1,
		Total:       total,
		TotalPages:  totalPages,
		HasNextPage: total > offset+limit,
		HasPrevPage: offset > 0,
	}
}
