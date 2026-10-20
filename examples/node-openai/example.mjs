import OpenAI from 'openai';

const client = new OpenAI({apiKey: process.env.OPENAI_API_KEY,
  baseURL: process.env.OPENAI_BASE_URL || 'https://api.openai.com/v1', maxRetries: 0});
const response = await client.chat.completions.create({
  model: process.env.OPENAI_MODEL || 'gpt-4.1', max_completion_tokens: 64,
  messages: [{role: 'user', content: 'Say hello in one short sentence.'}]
}, process.env.TOLLGLASS_TAG ? {headers: {'X-Tollglass-Tag': process.env.TOLLGLASS_TAG}} : {});
console.log(response.choices[0].message.content);
