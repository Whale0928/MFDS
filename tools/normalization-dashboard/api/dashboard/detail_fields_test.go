package dashboard

import (
	"regexp"
	"testing"
)

// declarationMatchingSourceSQL은 mfds_importers, alcohols, distilleries, regions를 함께 join한다.
// 아래 컬럼은 mfds_declaration_details와 join 대상 테이블 양쪽에 있어서
// 테이블 접두어 없이 쓰면 MySQL이 1052(ambiguous)로 질의를 거부한다.
var sharedJoinColumns = []string{"id", "created_at", "updated_at", "reviewed_at", "reviewed_by", "source_observed_at"}

func TestDetailColumns_공유컬럼을테이블접두어없이참조하지않는다(t *testing.T) {
	t.Parallel()

	for _, name := range sharedJoinColumns {
		pattern := regexp.MustCompile(`(^|[^.\w])` + name + `([^\w]|$)`)
		for _, column := range detailColumns {
			if pattern.MatchString(column.Expr) {
				t.Errorf("%s 필드의 %q가 %s를 접두어 없이 참조한다", column.Label, column.Expr, name)
			}
		}
	}
}
