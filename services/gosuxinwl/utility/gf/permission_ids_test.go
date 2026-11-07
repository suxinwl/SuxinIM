package gf

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

func TestPermissionIDsCanonicalJSONAndCSV(t *testing.T) {
	for _, test := range []struct {
		value string
		want  []string
	}{
		{"", []string{}}, {" \t ", []string{}}, {"[]", []string{}}, {"[ ]", []string{}},
		{"31", []string{"31"}}, {" 31 , 41 ", []string{"31", "41"}},
		{"[31]", []string{"31"}}, {`["31"]`, []string{"31"}}, {`[31,"41",51]`, []string{"31", "41", "51"}},
		{`["031",31," 041 ",41]`, []string{"31", "41"}}, {"031,31,41", []string{"31", "41"}},
		{"9223372036854775807", []string{"9223372036854775807"}}, {"*", []string{"*"}},
	} {
		t.Run(test.value, func(t *testing.T) {
			got, err := PermissionIDs(test.value)
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got=%v error=%v want=%v", got, err, test.want)
			}
		})
	}
}

func TestPermissionIDsRejectMalformedRowsWithoutPartialGrants(t *testing.T) {
	for _, value := range []string{
		"0", "-1", "+1", "1.0", "1e2", "9223372036854775808", "1,", ",1", "1,,2", "31]", "31 OR 1=1", "31abc", "１",
		"* ", " *", "1,*", `"31"`, `{"id":31}`, "null", "true", "[31", "[31],41", "[31,]",
		`[31,"*"]`, `[31,0]`, `[31,-1]`, `[31,1.0]`, `[31,1e2]`, `[31,null]`, `[31,true]`, `[31,{}]`, `[31,[41]]`, `[31,"41abc"]`, `[31,"4\n1"]`,
	} {
		t.Run(value, func(t *testing.T) {
			if got, err := PermissionIDs(value); err == nil || len(got) != 0 {
				t.Fatalf("malformed row became grants: %v error=%v", got, err)
			}
			if got := ArrayMerge([]*gvar.Var{gvar.New(value)}); len(got) != 0 {
				t.Fatalf("malformed row was merged: %v", got)
			}
		})
	}
}

func TestPermissionArrayMergeDeduplicatesRowsAndKeepsExactWildcard(t *testing.T) {
	got := ArrayMerge([]*gvar.Var{nil, gvar.New(""), gvar.New("[31]"), gvar.New("31,41"), gvar.New([]int{51, 31}), gvar.New(`"61"`), gvar.New("[71,invalid]"), gvar.New("*"), gvar.New("*")})
	want := []interface{}{"31", "41", "51", "*"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
	if got := ArrayMerge(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty result must be an empty permission list: %v", got)
	}
}

func TestPermissionRowsAreSafeDatabaseIDFilters(t *testing.T) {
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "permission-ids.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	if _, err = db.Exec(ctx, `CREATE TABLE rules(id INTEGER PRIMARY KEY);`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO rules VALUES(31),(41),(51)`); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		value string
		count int
	}{
		{"[31]", 1}, {`["31",41]`, 2}, {"31,41", 2}, {"", 0}, {"[]", 0}, {"31abc", 0}, {"[31,invalid]", 0}, {"*", 0},
	} {
		count, err := db.Model("rules").WhereIn("id", ArrayMerge([]*gvar.Var{gvar.New(test.value)})).Count()
		if err != nil || count != test.count {
			t.Fatalf("%q matched %d rows, want %d: %v", test.value, count, test.count, err)
		}
	}
}
