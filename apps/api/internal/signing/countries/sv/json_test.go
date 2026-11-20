package sv

import (
	"strings"
	"testing"
)

// Expected values are what JSON.stringify(JSON.parse(input)) gives in Node.
func TestRoundTrip(t *testing.T) {
	for in, want := range map[string]string{
		`[1.0, 1e21, 1e20, 1e-7, 1e-6, 0.1, -0, 5e-324, 1.7976931348623157e308, 1e400, -1e400, 123456789012345678901, 0.000001234, 1.5e-10, 100, 1E2, 25e-1]`: `[1,1e+21,100000000000000000000,1e-7,0.000001,0.1,0,5e-324,1.7976931348623157e+308,null,null,123456789012345680000,0.000001234,1.5e-10,100,100,2.5]`,
		`{"b":1,"a":2,"1":3,"0":4,"01":5,"4294967295":6,"4294967294":7,"-1":8}`:                                                                               `{"0":4,"1":3,"4294967294":7,"b":1,"a":2,"01":5,"4294967295":6,"-1":8}`,
		`{"a":1,"b":2,"a":3}`:                    `{"a":3,"b":2}`,
		`"\u00e9\u2028\u0000\u001f\u007f\/\"\\"`: "\"é \\u0000\\u001f\u007f/\\\"\\\\\"",
		`"\ud83d\ude00 \ud800 \udc00x"`:          `"😀 \ud800 \udc00x"`,
		` { "a" : [ true , false , null ] } `:    `{"a":[true,false,null]}`,
	} {
		v, err := parseJSON(in)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		var b strings.Builder
		stringify(&b, v)
		if b.String() != want {
			t.Errorf("%s\n got %s\nwant %s", in, b.String(), want)
		}
	}
	for _, bad := range []string{``, `{`, `[1,]`, `01`, `1.`, `.5`, `+1`, `"a`, `'a'`, `{a:1}`, `NaN`, `[1] [2]`, "\"\t\""} {
		if _, err := parseJSON(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
