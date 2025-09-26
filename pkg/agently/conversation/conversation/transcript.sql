SELECT
      t.*,
      0 elapsedInSec,
      '' AS stage
       FROM turn t
      ${predicate.Builder().CombineOr($predicate.FilterGroup(0, "AND")).Build("WHERE")}