SELECT * FROM user_oauth_token
WHERE $criteria.In("user_id", $Token.UserID) AND $criteria.In("provider", $Token.Provider)
