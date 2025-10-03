SELECT
      t.*,
      0 elapsedInSec,
      '' AS stage,
      '' AS toolFeed,
      0 AS isLast
       FROM turn t
      ${predicate.Builder().CombineOr($predicate.FilterGroup(1, "AND")).Build("WHERE")}