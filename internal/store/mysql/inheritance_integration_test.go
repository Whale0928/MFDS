package mysql

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/usecase/inheritance"
)

type inheritanceFixture struct {
	store    *Store
	base     *normalizationFixture
	key      string
	alcohols []int64
	now      time.Time
}

func newInheritanceFixture(t *testing.T) *inheritanceFixture {
	t.Helper()
	store := normalizationStore(t)
	base := newNormalizationFixture(t, store)
	var runIDs []int64
	cleanupMatchingChildren(t, store, &base.rcnos, &runIDs)
	fixture := &inheritanceFixture{
		store: store, base: base, now: time.Now().UTC().Truncate(time.Microsecond),
		key: fmt.Sprintf("%064x", time.Now().UnixNano()),
	}
	t.Cleanup(func() {
		for _, id := range fixture.alcohols {
			if _, err := store.db.Exec(`DELETE FROM alcohols WHERE id = ?`, id); err != nil {
				t.Errorf("alcohol cleanup failed: %v", err)
			}
		}
	})
	return fixture
}

func (f *inheritanceFixture) alcohol(t *testing.T, name string, distilleryID, regionID int64) int64 {
	t.Helper()
	id := 800000000 + time.Now().UnixNano()%100000000
	if _, err := f.store.db.Exec(`
		INSERT INTO alcohols (id, kor_name, eng_name, abv, type, kor_category, eng_category, category_group, region_id, distillery_id, age, volume)
		VALUES (?, ?, ?, '40', 'WHISKY', '위스키', 'Whisky', 'SINGLE_MALT', ?, ?, '12', '700')
	`, id, name, name, regionID, distilleryID); err != nil {
		t.Fatal(err)
	}
	f.alcohols = append(f.alcohols, id)
	return id
}

// row creates a normalized whisky declaration under the fixture key and applies the given selection columns.
func (f *inheritanceFixture) row(t *testing.T, label, decision string, alcoholID int64) (string, int64) {
	t.Helper()
	rcno := fmt.Sprintf("IH-%s-%d", label, time.Now().UnixNano())
	f.base.item(t, rcno, "inherit-"+rcno, f.now, f.now)
	if err := f.store.SyncDeclarations(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`
		UPDATE mfds_declarations
		SET normalization_status = 'NORMALIZED', normalized_at = ?, product_identity_key_sha256 = UNHEX(?),
		    alcohol_category_en = 'Whisky', manufacture_country_alpha2 = 'GB',
		    alcohol_match_decision = NULLIF(?, ''), selected_alcohol_id = NULLIF(?, 0)
		WHERE rcno = ?
	`, f.now, f.key, decision, alcoholID, rcno); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := f.store.db.QueryRow(`SELECT id FROM mfds_declarations WHERE rcno = ?`, rcno).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return rcno, id
}

func (f *inheritanceFixture) execute(t *testing.T, dryRun bool) inheritance.Summary {
	t.Helper()
	service, err := inheritance.NewService(f.store)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Execute(context.Background(), dryRun)
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

func (f *inheritanceFixture) selections(t *testing.T, rcno, action string) []string {
	t.Helper()
	rows, err := f.store.db.Query(`
		SELECT CONCAT(s.target_type, ':', s.target_id, ':', s.reason_code, ':', s.selected_by)
		FROM mfds_matching_selections AS s JOIN mfds_declarations AS d ON d.id = s.declaration_id
		WHERE d.rcno = ? AND s.selection_source = 'INHERITED' AND s.action = ? AND s.run_id IS NULL
		ORDER BY s.id
	`, rcno, action)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	return values
}

func TestInheritance_시드선택을같은키의미선택행에채우고재실행은바꾸지않는다(t *testing.T) {
	// Given
	fixture := newInheritanceFixture(t)
	alcoholID := fixture.alcohol(t, "inherit-jw-black", 150, 19)
	_, seedID := fixture.row(t, "S", "CANDIDATE", alcoholID)
	target, _ := fixture.row(t, "T", "REVIEW", 0)
	review, _ := fixture.row(t, "R", "REVIEW", 0)
	if _, err := fixture.store.db.Exec(`UPDATE mfds_declarations SET normalization_status = 'REVIEW_REQUIRED' WHERE rcno = ?`, review); err != nil {
		t.Fatal(err)
	}

	// When
	dryRun := fixture.execute(t, true)
	afterDryRun := readSelectionState(t, fixture.store, target)
	applied := fixture.execute(t, false)
	afterApply := readSelectionState(t, fixture.store, target)
	again := fixture.execute(t, false)

	// Then
	if dryRun.Inherited < 1 || afterDryRun.alcoholID.Valid {
		t.Fatalf("dry run = %+v, state = %+v", dryRun, afterDryRun)
	}
	if applied.Inherited < 1 || afterApply.decision.String != "INHERITED" || afterApply.alcoholID.Int64 != alcoholID ||
		afterApply.inheritedFrom.Int64 != seedID || afterApply.distilleryID.Int64 != 150 || afterApply.regionID.Int64 != 19 ||
		afterApply.distillerySource.String != "ALCOHOL_PROPAGATED" || afterApply.regionSource.String != "ALCOHOL_PROPAGATED" {
		t.Fatalf("applied = %+v, state = %+v", applied, afterApply)
	}
	if reviewState := readSelectionState(t, fixture.store, review); reviewState.alcoholID.Valid {
		t.Fatalf("REVIEW_REQUIRED target was filled: %+v", reviewState)
	}
	seedBy := fmt.Sprintf("declaration:%d", seedID)
	history := fixture.selections(t, target, "SELECT")
	if len(history) != 3 || history[0] != fmt.Sprintf("ALCOHOL:%d:INHERITED_FROM_DECLARATION:%s", alcoholID, seedBy) ||
		history[1] != "DISTILLERY:150:ALCOHOL_PROPAGATED:"+seedBy || history[2] != "REGION:19:ALCOHOL_PROPAGATED:"+seedBy {
		t.Fatalf("history = %v", history)
	}
	if again.Inherited != 0 || len(fixture.selections(t, target, "SELECT")) != 3 {
		t.Fatalf("second run = %+v", again)
	}
}

func TestInheritance_시드해제와재확정을상속행에전파한다(t *testing.T) {
	// Given
	fixture := newInheritanceFixture(t)
	first := fixture.alcohol(t, "inherit-first", 150, 19)
	second := fixture.alcohol(t, "inherit-second", 151, 20)
	seed, seedID := fixture.row(t, "S", "MANUAL", first)
	target, _ := fixture.row(t, "T", "REVIEW", 0)
	fixture.execute(t, false)

	// When: 관리자가 시드를 다른 알코올로 재확정
	if _, err := fixture.store.db.Exec(`UPDATE mfds_declarations SET selected_alcohol_id = ? WHERE rcno = ?`, second, seed); err != nil {
		t.Fatal(err)
	}
	reassigned := fixture.execute(t, false)
	afterReassign := readSelectionState(t, fixture.store, target)
	// When: 관리자가 시드를 해제
	if _, err := fixture.store.db.Exec(`
		UPDATE mfds_declarations
		SET alcohol_match_decision = NULL, selected_alcohol_id = NULL, selected_distillery_id = NULL,
		    distillery_match_source = NULL, selected_region_id = NULL, region_match_source = NULL
		WHERE rcno = ?
	`, seed); err != nil {
		t.Fatal(err)
	}
	released := fixture.execute(t, false)
	afterRelease := readSelectionState(t, fixture.store, target)

	// Then
	if reassigned.Inherited < 1 || afterReassign.alcoholID.Int64 != second || afterReassign.distilleryID.Int64 != 151 || afterReassign.inheritedFrom.Int64 != seedID {
		t.Fatalf("reassigned = %+v, state = %+v", reassigned, afterReassign)
	}
	if released.Released < 1 || afterRelease.decision.Valid || afterRelease.alcoholID.Valid || afterRelease.distilleryID.Valid || afterRelease.inheritedFrom.Valid {
		t.Fatalf("released = %+v, state = %+v", released, afterRelease)
	}
	revokes := fixture.selections(t, target, "REVOKE")
	if len(revokes) != 1 || revokes[0] != fmt.Sprintf("ALCOHOL:%d:%s:declaration:%d", second, inheritance.ReleaseSeedInvalid, seedID) {
		t.Fatalf("revokes = %v", revokes)
	}
}

func TestInheritance_읽은뒤관리자가확정한행과바뀐시드는덮어쓰지않는다(t *testing.T) {
	// Given
	fixture := newInheritanceFixture(t)
	alcoholID := fixture.alcohol(t, "inherit-concurrent", 150, 19)
	other := fixture.alcohol(t, "inherit-admin", 152, 21)
	seed, _ := fixture.row(t, "S", "CANDIDATE", alcoholID)
	target, _ := fixture.row(t, "T", "REVIEW", 0)
	rows, err := fixture.store.LoadInheritanceRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	alcohols, err := fixture.store.LoadInheritanceAlcohols(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan := inheritance.BuildPlan(rows, alcohols)
	var group inheritance.Group
	for _, candidate := range plan.Groups {
		if candidate.IdentityKey == fixture.key {
			group = candidate
		}
	}
	if len(group.Fills) != 1 {
		t.Fatalf("group = %+v", group)
	}

	// When: 계획을 읽은 뒤 api-server가 대상을 확정
	if _, err := fixture.store.db.Exec(`UPDATE mfds_declarations SET alcohol_match_decision = 'MANUAL', selected_alcohol_id = ? WHERE rcno = ?`, other, target); err != nil {
		t.Fatal(err)
	}
	result, applyErr := fixture.store.ApplyInheritanceGroup(context.Background(), group)
	// When: 시드가 다른 알코올로 재확정된 뒤 같은 계획을 적용
	if _, err := fixture.store.db.Exec(`UPDATE mfds_declarations SET alcohol_match_decision = NULL, selected_alcohol_id = NULL WHERE rcno = ?`, target); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.db.Exec(`UPDATE mfds_declarations SET selected_alcohol_id = ? WHERE rcno = ?`, other, seed); err != nil {
		t.Fatal(err)
	}
	seedChanged, seedErr := fixture.store.ApplyInheritanceGroup(context.Background(), group)

	// Then
	if applyErr != nil || result.Filled != 0 || result.Skipped != 1 {
		t.Fatalf("result = %+v, error = %v", result, applyErr)
	}
	if seedErr != nil || seedChanged.Skipped == 0 || seedChanged.Filled != 0 {
		t.Fatalf("seed changed result = %+v, error = %v", seedChanged, seedErr)
	}
	if state := readSelectionState(t, fixture.store, target); state.alcoholID.Valid || state.decision.Valid {
		t.Fatalf("target state = %+v", state)
	}
}

func TestInheritance_읽은뒤바뀐상속행은해제하지않는다(t *testing.T) {
	// Given
	fixture := newInheritanceFixture(t)
	alcoholID := fixture.alcohol(t, "inherit-release-fence", 150, 19)
	target, targetID := fixture.row(t, "T", "INHERITED", alcoholID)
	if _, err := fixture.store.db.Exec(`UPDATE mfds_declarations SET inherited_from_declaration_id = 1 WHERE rcno = ?`, target); err != nil {
		t.Fatal(err)
	}
	action := inheritance.Action{Current: inheritance.Row{
		DeclarationID: targetID, RCNO: target, Decision: "INHERITED", SelectedAlcoholID: alcoholID, InheritedFromDeclarationID: 1,
	}, Reason: inheritance.ReleaseSeedInvalid}

	// When: 관리자가 같은 행을 직접 확정
	if _, err := fixture.store.db.Exec(`UPDATE mfds_declarations SET alcohol_match_decision = 'CANDIDATE', inherited_from_declaration_id = NULL WHERE rcno = ?`, target); err != nil {
		t.Fatal(err)
	}
	released, err := fixture.store.ReleaseInheritance(context.Background(), action)

	// Then
	if err != nil || released {
		t.Fatalf("released = %t, error = %v", released, err)
	}
	if state := readSelectionState(t, fixture.store, target); state.decision.String != "CANDIDATE" || state.alcoholID.Int64 != alcoholID {
		t.Fatalf("state = %+v", state)
	}
}

func TestInheritance_관리자확정이같은키의자동확정행을덮어쓴다(t *testing.T) {
	// Given
	fixture := newInheritanceFixture(t)
	admin := fixture.alcohol(t, "inherit-admin", 150, 19)
	other := fixture.alcohol(t, "inherit-other", 151, 20)
	_, seedID := fixture.row(t, "S", "MANUAL", admin)
	target, _ := fixture.row(t, "T", "AUTO_SELECTED", other)

	// When
	applied := fixture.execute(t, false)
	state := readSelectionState(t, fixture.store, target)

	// Then
	if applied.Inherited != 1 || state.decision.String != "INHERITED" || state.alcoholID.Int64 != admin ||
		state.inheritedFrom.Int64 != seedID || state.distilleryID.Int64 != 150 || state.regionID.Int64 != 19 {
		t.Fatalf("applied = %+v, state = %+v", applied, state)
	}
}

func TestApplyMatchedAlcoholNames_매칭된행의알코올명만알코올이름으로바꾼다(t *testing.T) {
	// Given
	fixture := newInheritanceFixture(t)
	alcohol := fixture.alcohol(t, "names-alcohol", 150, 19)
	if _, err := fixture.store.db.Exec(`UPDATE alcohols SET kor_name = '글렌모렌지 스피오스', eng_name = 'Glenmorangie Spios' WHERE id = ?`, alcohol); err != nil {
		t.Fatal(err)
	}
	matched, _ := fixture.row(t, "M", "AUTO_SELECTED", alcohol)
	unmatched, _ := fixture.row(t, "U", "REVIEW", 0)
	for _, rcno := range []string{matched, unmatched} {
		if _, err := fixture.store.db.Exec(`
			UPDATE mfds_declarations
			SET alcohol_name_ko = '글렌모렌지 프라이빗에디션 스피오스', alcohol_name_en = 'GLENMORANGIE PRIVATE EDITION SPIOS',
			    base_product_name_ko = '글렌모렌지 프라이빗에디션 스피오스', name_search_key_ko = '글렌모렌지 프라이빗에디션 스피오스', sku_display_name_ko = '글렌모렌지 프라이빗에디션 스피오스 700ml'
			WHERE rcno = ?
		`, rcno); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()

	// When
	dryRun, dryErr := fixture.store.ApplyMatchedAlcoholNames(ctx, true)
	applied, applyErr := fixture.store.ApplyMatchedAlcoholNames(ctx, false)
	again, againErr := fixture.store.ApplyMatchedAlcoholNames(ctx, false)

	// Then
	if dryErr != nil || applyErr != nil || againErr != nil || dryRun < 1 || applied < 1 || again != 0 {
		t.Fatalf("dry = %d/%v, applied = %d/%v, again = %d/%v", dryRun, dryErr, applied, applyErr, again, againErr)
	}
	read := func(rcno string) (string, string, string, string, string) {
		var ko, en, search, base, key string
		if err := fixture.store.db.QueryRow(`
			SELECT alcohol_name_ko, alcohol_name_en, name_search_key_ko, base_product_name_ko,
			       LOWER(HEX(product_identity_key_sha256))
			FROM mfds_declarations WHERE rcno = ?
		`, rcno).Scan(&ko, &en, &search, &base, &key); err != nil {
			t.Fatal(err)
		}
		return ko, en, search, base, key
	}
	ko, en, search, base, key := read(matched)
	if ko != "글렌모렌지 스피오스" || en != "Glenmorangie Spios" || search != "글렌모렌지 프라이빗에디션 스피오스" ||
		base != "글렌모렌지 프라이빗에디션 스피오스" || key != fixture.key {
		t.Fatalf("matched row = %q %q %q %q %q", ko, en, search, base, key)
	}
	if ko, _, _, _, _ := read(unmatched); ko != "글렌모렌지 프라이빗에디션 스피오스" {
		t.Fatalf("unmatched row renamed: %q", ko)
	}
}
