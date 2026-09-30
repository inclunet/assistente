const chatGPTKeys: Record<string, string> = {
  chatgpt_response_incomplete: 'chatgpt.errors.incomplete',
  chatgpt_stream_interrupted: 'chatgpt.errors.interrupted',
  chatgpt_plan_limit: 'chatgpt.errors.planLimit',
  chatgpt_model_unavailable: 'chatgpt.errors.modelUnavailable',
  chatgpt_reauthorization_required: 'chatgpt.errors.reauthorize',
  chatgpt_permission_required: 'chatgpt.errors.permission',
  chatgpt_rate_limit: 'chatgpt.errors.rateLimit',
  chatgpt_request_cancelled: 'chatgpt.errors.cancelled',
  chatgpt_temporarily_unavailable: 'chatgpt.errors.temporary',
  chatgpt_request_failed: 'chatgpt.errors.failed',
};

export function chatGPTErrorKey(message: string): string | undefined {
  return Object.prototype.hasOwnProperty.call(chatGPTKeys, message) ? chatGPTKeys[message] : undefined;
}
