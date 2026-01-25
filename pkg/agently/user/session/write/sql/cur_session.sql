SELECT * FROM session
WHERE $criteria.In("id", $Session.Id)
