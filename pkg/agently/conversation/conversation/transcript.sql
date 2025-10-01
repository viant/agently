SELECT
      t.*,
      0 elapsedInSec,
      '' AS stage,
      '' AS toolFeed
       FROM turn t
      ${predicate.Builder().CombineOr($predicate.FilterGroup(1, "AND")).Build("WHERE")}