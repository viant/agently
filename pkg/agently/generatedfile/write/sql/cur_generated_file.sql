SELECT * FROM generated_file
WHERE $criteria.In("id", $CurIDs.Values)
