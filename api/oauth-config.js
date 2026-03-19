// api/oauth-config.js

const RATE_LIMIT = new Map(); // ip -> { count, resetAt }
const MAX_REQUESTS = 10;
const WINDOW_MS = 24 * 60 * 60 * 1000; // 24 hours

function checkRateLimit(ip) {
  const now = Date.now();
  const entry = RATE_LIMIT.get(ip);

  if (!entry || now > entry.resetAt) {
    RATE_LIMIT.set(ip, { count: 1, resetAt: now + WINDOW_MS });
    return true;
  }
  if (entry.count >= MAX_REQUESTS) return false;
  entry.count++;
  return true;
}

export default function handler(req, res) {
  // Only allow GET
  if (req.method !== 'GET') {
    return res.status(405).json({ error: 'Method not allowed' });
  }

  // Rate limit by IP
  const ip = req.headers['x-forwarded-for']?.split(',')[0] || req.socket.remoteAddress || 'unknown';
  if (!checkRateLimit(ip)) {
    return res.status(429).json({ error: 'Rate limit exceeded — try again tomorrow' });
  }

  // Optional: require a shared secret header so only your binary can call this
  // Set CLAW_API_SECRET in Vercel env vars and in your Go binary
  const apiSecret = process.env.CLAW_API_SECRET;
  if (apiSecret) {
    const provided = req.headers['x-claw-secret'];
    if (!provided || provided !== apiSecret) {
      return res.status(401).json({ error: 'Unauthorized' });
    }
  }

  const clientId = process.env.GOOGLE_CLIENT_ID;
  const clientSecret = process.env.GOOGLE_CLIENT_SECRET;

  if (!clientId || !clientSecret) {
    return res.status(500).json({ error: 'OAuth config not set on server' });
  }

  // Cache-control: no caching — always fetch fresh
  res.setHeader('Cache-Control', 'no-store');
  res.status(200).json({
    client_id: clientId,
    client_secret: clientSecret,
  });
}
