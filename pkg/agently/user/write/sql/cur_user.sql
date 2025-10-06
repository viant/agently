SELECT * FROM users
WHERE $criteria.In("id", $CurUsersId.Values)
