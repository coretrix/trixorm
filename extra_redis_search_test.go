package trixorm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtraRedisSearchQueryBuilderScenarios(t *testing.T) {
	search := extraRedisSearchForBuilder(t, "extra_query")

	tests := []struct {
		name     string
		query    *RedisSearchQuery
		expected []interface{}
	}{
		{
			name:     "empty query becomes wildcard",
			query:    NewRedisSearchQuery(),
			expected: []interface{}{"FT.SEARCH", "idx", "*"},
		},
		{
			name:     "raw query is preserved",
			query:    NewRedisSearchQuery().QueryRaw("(@Age:[1 3])"),
			expected: []interface{}{"FT.SEARCH", "idx", "(@Age:[1 3])"},
		},
		{
			name:     "plain query is escaped",
			query:    NewRedisSearchQuery().Query("alpha+beta:(1)"),
			expected: []interface{}{"FT.SEARCH", "idx", `alpha\+beta\:\(1\)`},
		},
		{
			name:     "numeric and tag filters keep insertion order",
			query:    NewRedisSearchQuery().FilterIntMinMax("Age", 10, 20).FilterTag("Status", "active", "queued"),
			expected: []interface{}{"FT.SEARCH", "idx", "@Age:[10 20] @Status:{ active | queued }"},
		},
		{
			name:     "multiple exact string values join with OR",
			query:    NewRedisSearchQuery().FilterString("Name", "Ada Lovelace", "Grace Hopper"),
			expected: []interface{}{"FT.SEARCH", "idx", `@Name:( "Ada Lovelace" | "Grace Hopper" )`},
		},
		{
			name:     "not string is appended after positive filters",
			query:    NewRedisSearchQuery().FilterTag("Status", "active").FilterNotString("Name", "Archived"),
			expected: []interface{}{"FT.SEARCH", "idx", `@Status:{ active } -@Name:( "Archived" )`},
		},
		{
			name:     "prefix query ignores one letter words",
			query:    NewRedisSearchQuery().QueryFieldPrefixMatch("Name", "a alpha be"),
			expected: []interface{}{"FT.SEARCH", "idx", "@Name:( alpha* be* )"},
		},
		{
			name:     "raw field query does not quote the value",
			query:    NewRedisSearchQuery().QueryField("Code", "mk-100"),
			expected: []interface{}{"FT.SEARCH", "idx", `@Code:( mk\-100 )`},
		},
		{
			name:     "not numeric builds exclusion ranges",
			query:    NewRedisSearchQuery().FilterNotInt("Age", 10),
			expected: []interface{}{"FT.SEARCH", "idx", "(@Age:[-inf (10] | @Age:[(10 +inf])"},
		},
		{
			name:     "uint greater and less filters are numeric ranges",
			query:    NewRedisSearchQuery().FilterUintGreaterEqual("Amount", 100).FilterUintLess("Amount", 200),
			expected: []interface{}{"FT.SEARCH", "idx", "@Amount:[100 +inf]|@Amount:[-inf (200]"},
		},
		{
			name:     "float exact filter pads precision",
			query:    NewRedisSearchQuery().FilterFloat("Score", 7.5),
			expected: []interface{}{"FT.SEARCH", "idx", "@Score:[7.49999 7.50001]"},
		},
		{
			name:     "empty tag maps to NULL token",
			query:    NewRedisSearchQuery().FilterTag("Status", ""),
			expected: []interface{}{"FT.SEARCH", "idx", "@Status:{ NULL }"},
		},
		{
			name:     "empty string maps to quoted NULL token",
			query:    NewRedisSearchQuery().FilterString("Name", ""),
			expected: []interface{}{"FT.SEARCH", "idx", `@Name:( "NULL" )`},
		},
		{
			name:     "not tag is negated",
			query:    NewRedisSearchQuery().FilterNotTag("Status", "deleted", "archived"),
			expected: []interface{}{"FT.SEARCH", "idx", "-@Status:{ deleted | archived }"},
		},
		{
			name:     "bool filter maps to tag values",
			query:    NewRedisSearchQuery().FilterBool("Enabled", false),
			expected: []interface{}{"FT.SEARCH", "idx", "@Enabled:{ false }"},
		},
		{
			name:     "many reference filter uses entity tokens",
			query:    NewRedisSearchQuery().FilterManyReferenceIn("Children", 1, 22),
			expected: []interface{}{"FT.SEARCH", "idx", "@Children:( e1 | e22 )"},
		},
		{
			name:     "many reference negative filter uses entity tokens",
			query:    NewRedisSearchQuery().FilterManyReferenceNotIn("Children", 1, 22),
			expected: []interface{}{"FT.SEARCH", "idx", "-@Children:( e1 | e22 )"},
		},
		{
			name:     "fake delete filter is added by default",
			query:    extraRedisSearchQueryWithFakeDelete(NewRedisSearchQuery()),
			expected: []interface{}{"FT.SEARCH", "idx", "-@FakeDelete:{true}"},
		},
		{
			name:     "fake deleted rows can be included",
			query:    extraRedisSearchQueryWithFakeDelete(NewRedisSearchQuery().WithFakeDeleteRows()),
			expected: []interface{}{"FT.SEARCH", "idx", "*"},
		},
		{
			name:     "raw suffix is appended after filters",
			query:    NewRedisSearchQuery().FilterTag("Status", "active").AppendQueryRawAfterFilters("@Name:extra"),
			expected: []interface{}{"FT.SEARCH", "idx", "@Status:{ active } @Name:extra"},
		},
		{
			name:     "geo filter is emitted as command arguments",
			query:    NewRedisSearchQuery().FilterGeo("Location", 21.43, 41.99, 5, "km"),
			expected: []interface{}{"FT.SEARCH", "idx", "*", "GEOFILTER", "Location", 21.43, 41.99, float64(5), "km"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := search.buildQueryArgsOrdered(test.query, []interface{}{"FT.SEARCH", "idx"})

			assert.Equal(t, test.expected, args)
		})
	}

	t.Run("prefix query requires one searchable word", func(t *testing.T) {
		assert.PanicsWithError(t, "search start with requires min one word with 2 characters", func() {
			NewRedisSearchQuery().QueryFieldPrefixMatch("Name", "a b")
		})
	})
}

func TestExtraRedisSearchCommandArgumentScenarios(t *testing.T) {
	search := extraRedisSearchForBuilder(t, "extra_cmd")

	t.Run("search options are appended around pager", func(t *testing.T) {
		query := NewRedisSearchQuery().
			QueryRaw("hello").
			Verbatim().
			NoStopWords().
			WithScores().
			Sort("Age", true).
			InKeys("k1", "k2").
			InFields("Name", "Description").
			Return("Name", "Age").
			Slop(0).
			InOrder().
			Lang("english").
			Highlight("Name").
			HighlightTags("<b>", "</b>").
			Summarize("Description").
			SummarizeOptions("...", 2, 20)

		args := []interface{}{"FT.SEARCH", "extra_cmd:idx"}
		args = search.buildQueryArgsOrdered(query, args)
		args = append(args, "VERBATIM", "NOSTOPWORDS", "WITHSCORES", "SORTBY", "Age", "DESC")
		args = append(args, "INKEYS", 2, "extra_cmd:k1", "extra_cmd:k2")
		args = append(args, "INFIELDS", 2, "Name", "Description")
		args = append(args, "RETURN", 2, "Name", "Age")
		args = append(args, "SLOP", 0, "INORDER", "LANGUAGE", "english")
		args = append(args, "HIGHLIGHT", "FIELDS", 1, "Name", "TAGS", "<b>", "</b>")
		args = append(args, "SUMMARIZE", "FIELDS", 1, "Description", "FRAGS", 2, "LEN", 20, "SEPARATOR", "...")
		args = search.applyPager(NewPager(2, 25), args)

		assert.Equal(t, []interface{}{
			"FT.SEARCH", "extra_cmd:idx", "hello",
			"VERBATIM", "NOSTOPWORDS", "WITHSCORES", "SORTBY", "Age", "DESC",
			"INKEYS", 2, "extra_cmd:k1", "extra_cmd:k2",
			"INFIELDS", 2, "Name", "Description",
			"RETURN", 2, "Name", "Age",
			"SLOP", 0, "INORDER", "LANGUAGE", "english",
			"HIGHLIGHT", "FIELDS", 1, "Name", "TAGS", "<b>", "</b>",
			"SUMMARIZE", "FIELDS", 1, "Description", "FRAGS", 2, "LEN", 20, "SEPARATOR", "...",
			"LIMIT", 25, 25,
		}, args)
	})

	t.Run("apply pager rejects missing pager", func(t *testing.T) {
		assert.PanicsWithError(t, "missing pager in redis search query", func() {
			search.applyPager(nil, []interface{}{"FT.SEARCH"})
		})
	})

	t.Run("apply pager rejects very large page size", func(t *testing.T) {
		assert.PanicsWithError(t, "pager size exceeded limit 10000", func() {
			search.applyPager(NewPager(1, 10001), []interface{}{"FT.SEARCH"})
		})
	})

	t.Run("index creation includes namespace options and fields", func(t *testing.T) {
		index := NewRedisSearchIndex("idx", "search", []string{"prefix:one:", "prefix:two:"})
		index.DefaultLanguage = "english"
		index.LanguageField = "Lang"
		index.DefaultScore = 0.5
		index.ScoreField = "Score"
		index.MaxTextFields = true
		index.NoOffsets = true
		index.NoNHL = true
		index.NoFields = true
		index.NoFreqs = true
		index.SkipInitialScan = true
		index.StopWords = []string{"a", "the"}
		index.AddTextField("Name", 2, true, false, true)
		index.AddNumericField("Age", true, true)
		index.AddGeoField("Location", false, false)
		index.AddTagField("Status", false, false, "|")

		assert.Equal(t, []interface{}{
			"FT.CREATE", "extra_cmd:idx", "ON", "HASH", "PREFIX", 2, "extra_cmd:prefix:one:", "extra_cmd:prefix:two:",
			"LANGUAGE", "english", "LANGUAGE_FIELD", "Lang", "SCORE", 0.5, "SCORE_FIELD", "Score",
			"MAXTEXTFIELDS", "NOOFFSETS", "NOHL", "NOFIELDS", "NOFREQS", "SKIPINITIALSCAN", "STOPWORDS", 2, "a", "the",
			"SCHEMA",
			"Name", "TEXT", "NOSTEM", "WEIGHT", float64(2), "SORTABLE",
			"Age", "NUMERIC", "SORTABLE", "NOINDEX",
			"Location", "GEO",
			"Status", "TAG", "SEPARATOR", "|",
		}, search.createIndexArgs(index, "idx"))
	})

	t.Run("index creation requires at least one prefix", func(t *testing.T) {
		index := NewRedisSearchIndex("idx", "search", nil)
		assert.PanicsWithError(t, "missing redis search prefix", func() {
			search.createIndexArgs(index, "idx")
		})
	})
}

func TestExtraRedisSearchAggregateBuilderScenarios(t *testing.T) {
	t.Run("load fields supports aliases", func(t *testing.T) {
		fields := &LoadFields{}
		fields.AddField("@Name")
		fields.AddFieldWithAlias("@Age", "age")

		assert.Equal(t, []string{"@Name", "@Age", "AS", "age"}, fields.args)
	})

	t.Run("aggregate chains load group sort apply and filter", func(t *testing.T) {
		fields := &LoadFields{}
		fields.AddField("@Status")
		aggregate := NewRedisSearchQuery().QueryRaw("*").Aggregate().
			Load(fields).
			GroupByField("@Status", NewAggregateReduceCount("count")).
			Sort(RedisSearchAggregateSort{Field: "@count", Desc: true}).
			Apply("@count * 2", "double_count").
			Filter("@double_count > 1")

		assert.Equal(t, []interface{}{
			"LOAD", "1", "@Status",
			"GROUPBY", 1, "@Status", "REDUCE", "COUNT", 0, "AS", "count",
			"SORTBY", "2", "@count", "DESC",
			"APPLY", "@count * 2", "AS", "double_count",
			"FILTER", "@double_count > 1",
		}, aggregate.args)
	})

	reducers := []struct {
		name     string
		reducer  AggregateReduce
		function string
		args     []interface{}
		alias    string
	}{
		{"count", NewAggregateReduceCount("count"), "COUNT", nil, "count"},
		{"count distinct", NewAggregateReduceCountDistinct("@City", "cities", false), "COUNT_DISTINCT", []interface{}{"@City"}, "cities"},
		{"count distinctish", NewAggregateReduceCountDistinct("@City", "cities", true), "COUNT_DISTINCTISH", []interface{}{"@City"}, "cities"},
		{"sum", NewAggregateReduceSum("@Amount", "amount"), "SUM", []interface{}{"@Amount"}, "amount"},
		{"min", NewAggregateReduceMin("@Amount", "min_amount"), "MIN", []interface{}{"@Amount"}, "min_amount"},
		{"max", NewAggregateReduceMax("@Amount", "max_amount"), "MAX", []interface{}{"@Amount"}, "max_amount"},
		{"avg", NewAggregateReduceAvg("@Amount", "avg_amount"), "AVG", []interface{}{"@Amount"}, "avg_amount"},
		{"stddev", NewAggregateReduceStdDev("@Amount", "std_amount"), "STDDEV", []interface{}{"@Amount"}, "std_amount"},
		{"quantile", NewAggregateReduceQuantile("@Amount", "0.95", "p95"), "QUANTILE", []interface{}{"@Amount", "0.95"}, "p95"},
		{"tolist", NewAggregateReduceToList("@Name", "names"), "TOLIST", []interface{}{"@Name"}, "names"},
		{"first value", NewAggregateReduceFirstValue("@Name", "first_name"), "FIRST_VALUE", []interface{}{"@Name"}, "first_name"},
		{"first value by desc", NewAggregateReduceFirstValueBy("@Name", "@Age", "oldest", true), "FIRST_VALUE", []interface{}{"@Name", "BY", "@Age", "DESC"}, "oldest"},
		{"random sample default", NewAggregateReduceRandomSample("@Name", "sample"), "RANDOM_SAMPLE", []interface{}{"@Name", "1"}, "sample"},
		{"random sample sized", NewAggregateReduceRandomSample("@Name", "sample", 5), "RANDOM_SAMPLE", []interface{}{"@Name", "5"}, "sample"},
	}

	for _, test := range reducers {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.function, test.reducer.function)
			assert.Equal(t, test.args, test.reducer.args)
			assert.Equal(t, test.alias, test.reducer.alias)
		})
	}

	t.Run("result value unescapes stored redis search text", func(t *testing.T) {
		result := &RedisSearchResult{Fields: []interface{}{
			"Name", EscapeRedisSearchString("a,b.c"),
			"MissingValue", "untouched",
		}}

		assert.Equal(t, "a,b.c", result.Value("Name"))
		assert.Nil(t, result.Value("Unknown"))
	})
}

func extraRedisSearchForBuilder(t *testing.T, namespace string) *RedisSearch {
	t.Helper()

	registry := NewRegistry()
	registry.RegisterRedis("localhost:6382", namespace, 0, "search")
	validatedRegistry, def, err := registry.Validate()
	require.NoError(t, err)
	t.Cleanup(def)

	return validatedRegistry.CreateEngine().GetRedisSearch("search")
}

func extraRedisSearchQueryWithFakeDelete(query *RedisSearchQuery) *RedisSearchQuery {
	query.hasFakeDelete = true
	return query
}
