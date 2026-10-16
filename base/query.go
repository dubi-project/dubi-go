package base

const (
	ASC    = 1
	DESC   = -1
	BYASC  = "asc"
	BYDESC = "desc"
)

const (
	OpAnd       = "$and"
	OpOr        = "$or"
	OpNor       = "$nor"
	OpNot       = "$not"
	OpEq        = "$eq"
	OpNe        = "$ne"
	OpGt        = "$gt"
	OpGte       = "$gte"
	OpLt        = "$lt"
	OpLte       = "$lte"
	OpIn        = "$in"
	OpNin       = "$nin"
	OpLike      = "$like"
	OpILike     = "$ilike"
	OpRegex     = "$regex"
	OpContains  = "$contains"
	OpOverlap   = "$overlap"
	OpElemMatch = "$elemMatch"
	OpExists    = "$exists"
	OpNull      = "$null"
	OpRange     = "$range"
)

const (
	OptSelect      = "$select"
	OptFields      = "$fields"
	OptSort        = "$sort"
	OptSorts       = "$sorts"
	OptLimit       = "$limit"
	OptOffset      = "$offset"
	OptAfter       = "$after"
	OptWithCount   = "$withCount"
	OptGroup       = "$group"
	OptHaving      = "$having"
	OptJoin        = "$join"
	OptAgg         = "$agg"
	OptUnsafe      = "$unsafe"
	OptBatch       = "$batch"
	OptWithDeleted = "$withDeleted"
	OptOnlyDeleted = "$onlyDeleted"
	OptUnscoped    = "$unscoped"
	OptFilter      = "$filter"
	OptFilters     = "$filters"
	OptKeyword     = "$keyword"
	OptQuery       = "$q"
	OptPrefix      = "$prefix"
	OptFacets      = "$facets"
	OptHighlight   = "$highlight"
	OptRaw         = "$raw"
	OptSetting     = "$setting"
)

const (
	UpdSet       = "$set"
	UpdInc       = "$inc"
	UpdUnset     = "$unset"
	UpdPush      = "$push"
	UpdPull      = "$pull"
	UpdAddToSet  = "$addToSet"
	UpdSetPath   = "$setPath"
	UpdUnsetPath = "$unsetPath"
)
