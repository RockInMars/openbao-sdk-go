package bao

import (
	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/kv"
	"github.com/RockInMars/openbao-sdk-go/pki"
	"github.com/RockInMars/openbao-sdk-go/transit"
	"reflect"
	"strings"
	"testing"
)

func TestPublicTypeBoundary(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var visit func(reflect.Type)
	visit = func(v reflect.Type) {
		if v == nil || seen[v] {
			return
		}
		seen[v] = true
		for _, forbidden := range []string{"github.com/openbao/", "gorm.io", "github.com/gin-gonic", "saas-go/services", "/internal/"} {
			if strings.Contains(v.PkgPath(), forbidden) {
				t.Fatal("public type crosses boundary")
			}
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				f := v.Field(i)
				if f.IsExported() {
					visit(f.Type)
				}
			}
		case reflect.Ptr, reflect.Slice, reflect.Array:
			visit(v.Elem())
		case reflect.Map:
			visit(v.Key())
			visit(v.Elem())
		case reflect.Interface:
			for i := 0; i < v.NumMethod(); i++ {
				visit(v.Method(i).Type)
			}
		case reflect.Func:
			for i := 0; i < v.NumIn(); i++ {
				visit(v.In(i))
			}
			for i := 0; i < v.NumOut(); i++ {
				visit(v.Out(i))
			}
		}
	}
	for _, v := range []any{Config{}, auth.Config{}, kv.ReadResult{}, kv.Metadata{}, pki.IssuedCertificate{}, pki.SignCSRRequest{}, transit.SignResult{}, transit.DecryptResult{}} {
		visit(reflect.TypeOf(v))
	}
	if _, ok := reflect.TypeOf(pki.SignedCertificate{}).FieldByName("PrivateKey"); ok {
		t.Fatal("CSR result must not have private key")
	}
}
