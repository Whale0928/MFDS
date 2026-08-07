package mysql

import "strings"

// columnAssignments는 UPDATE의 SET 절과 인자를 한 번에 모은다.
//
// 컬럼 목록과 인자 목록을 따로 쓰면 둘 사이 순서로만 연결되어, 같은 타입끼리
// 자리를 바꿔도 컴파일과 제약 검사를 모두 통과한 채 엉뚱한 컬럼에 값이 들어간다.
// 컬럼 이름과 값을 같은 호출에 묶어 그 실수를 구조적으로 막는다.
type columnAssignments struct {
	clauses []string
	args    []any
}

func newColumnAssignments(capacity int) *columnAssignments {
	return &columnAssignments{
		clauses: make([]string, 0, capacity),
		args:    make([]any, 0, capacity),
	}
}

// set은 컬럼 하나에 값 하나를 대입한다.
func (a *columnAssignments) set(column string, value any) {
	a.clauses = append(a.clauses, column+" = ?")
	a.args = append(a.args, value)
}

// setExpression은 상수나 CASE처럼 값 하나로 표현할 수 없는 대입을 받는다.
// expression에 포함된 자리표시자 수와 args 수는 호출자가 맞춘다.
func (a *columnAssignments) setExpression(expression string, args ...any) {
	a.clauses = append(a.clauses, expression)
	a.args = append(a.args, args...)
}

// clause는 SET 뒤에 붙일 대입 목록을 만든다.
func (a *columnAssignments) clause() string {
	return strings.Join(a.clauses, ",\n\t\t    ")
}

// arguments는 SET 절 인자에 WHERE 절 인자를 이어 붙인다.
func (a *columnAssignments) arguments(whereArgs ...any) []any {
	return append(append([]any{}, a.args...), whereArgs...)
}
