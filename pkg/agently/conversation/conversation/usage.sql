SELECT 
    m.conversation_id AS conversation_id,
    SUM(COALESCE(mc.cost, 0))                                           AS cost,
    SUM(COALESCE(mc.prompt_tokens, 0))                                  AS prompt_tokens,
    SUM(COALESCE(mc.prompt_cached_tokens, 0))                           AS prompt_cached_tokens,
    SUM(COALESCE(mc.prompt_audio_tokens, 0))                            AS prompt_audio_tokens,
    SUM(COALESCE(mc.completion_tokens, 0))                               AS completion_tokens,
    SUM(COALESCE(mc.completion_reasoning_tokens, 0))                    AS completion_reasoning_tokens,
    SUM(COALESCE(mc.completion_audio_tokens, 0))                         AS completion_audio_tokens,
    SUM(COALESCE(mc.completion_accepted_prediction_tokens, 0))          AS completion_accepted_prediction_tokens,
    SUM(COALESCE(mc.completion_rejected_prediction_tokens, 0))          AS completion_rejected_prediction_tokens,
    SUM(COALESCE(mc.total_tokens, 0))                                    AS total_tokens
  FROM model_call mc
  JOIN message m ON m.id = mc.message_id
  $View.ParentJoinOn("WHERE","m.conversation_id")
  GROUP BY m.conversation_id