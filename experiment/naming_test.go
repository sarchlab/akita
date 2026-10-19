package experiment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testNaming() *Naming {
	return &Naming{
		Separator: "_",
		Fields: []NamingField{
			{Name: "date", Pattern: `\d{4}`},
			{Name: "category", Values: map[string]string{"abl": "ablation", "abl-spmv": "ablation spmv", "diag": "diagnosis"}},
			{Name: "item", Pattern: `[A-Za-z0-9][A-Za-z0-9.-]*`, Not: []string{"all"}},
			{Name: "config", Pattern: `[a-z0-9]+(?:\+[a-z0-9]+)*`, VariantSeparator: "+"},
			{Name: "version", Pattern: `[0-9a-f]{7}`},
		},
	}
}

func TestParseName(t *testing.T) {
	n := testNaming()

	got := ParseName("1015_abl_wg32k_base+mshr128_3e9a41c", n)
	assert.Equal(t, map[string]string{
		"date": "1015", "category": "abl", "item": "wg32k",
		"config": "base+mshr128", "version": "3e9a41c",
	}, got)

	got = ParseName("1015_abl-spmv_x_base_3e9a41c", n)
	require.NotNil(t, got)
	assert.Equal(t, "abl-spmv", got["category"])

	assert.Nil(t, ParseName("1015_abl_all_base_3e9a41c", n), "excluded by not")
	assert.Nil(t, ParseName("1015_xyz_wg_base_3e9a41c", n), "value not in vocabulary")
	assert.Nil(t, ParseName("1015_abl_wg_base", n), "too few fields")
	assert.Nil(t, ParseName("anything", nil))
}

func TestParseNameValuesWithVariant(t *testing.T) {
	n := &Naming{Separator: "_", Fields: []NamingField{
		{Name: "config", Values: map[string]string{"base": "", "wide": ""}, VariantSeparator: "+", Not: []string{"wide"}},
		{Name: "v", Pattern: `\d+`},
	}}

	assert.Equal(t, map[string]string{"config": "base+l2x", "v": "1"}, ParseName("base+l2x_1", n))
	assert.Nil(t, ParseName("wide+l2x_1", n), "base value excluded")
	assert.Equal(t, "base", baseValue(n.Fields[0], "base+l2x"))
}

func TestCheckName(t *testing.T) {
	n := testNaming()

	require.NoError(t, CheckName("1015_diag_x_base_3e9a41c", n))

	err := CheckName("1015_diag_x_base", n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "want 5 fields")

	err = CheckName("1015_nope_x_base_3e9a41c", n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field category")

	err = CheckName("1015_abl_all_base_3e9a41c", n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")

	require.Error(t, CheckName("x", nil))
}

func TestFormatName(t *testing.T) {
	n := testNaming()

	name, err := formatName(map[string]string{
		"date": "1015", "category": "abl", "item": "wg", "config": "base", "version": "3e9a41c",
	}, n)
	require.NoError(t, err)
	assert.Equal(t, "1015_abl_wg_base_3e9a41c", name)

	_, err = formatName(map[string]string{"date": "1015"}, n)
	require.Error(t, err)

	_, err = formatName(map[string]string{
		"date": "15", "category": "abl", "item": "wg", "config": "base", "version": "3e9a41c",
	}, n)
	require.Error(t, err)
}

func TestParseProjectConfigRejectsBadNaming(t *testing.T) {
	_, err := parseProjectConfig([]byte(`{"schema":"exp-project/1",
		"naming":{"separator":"_","fields":[{"name":"a","pattern":"("}]}}`))
	require.Error(t, err)

	cfg, err := parseProjectConfig([]byte(`{"schema":"exp-project/1"}`))
	require.NoError(t, err)
	assert.Equal(t, defaultCampaignDetect, cfg.CampaignDetect())
	assert.Equal(t, "runs", cfg.CampaignAdapter().Type)
}
