SELECT user_id, provider, updated_at
FROM user_oauth_token
WHERE ($UserID = '' OR user_id = $UserID)
  AND ($Provider = '' OR provider = $Provider)
ORDER BY provider

