# Content Deploy Setup

How to configure automatic content deployment from a separate Git repository.

## Prerequisites
- A running BlogFlow instance with `sync.strategy: webhook` configured
- A separate Git repository for your blog content
- The webhook secret set via `BLOGFLOW_WEBHOOK_SECRET` env var on the server

## Setup Steps

### 1. Configure BlogFlow for webhook sync
In your BlogFlow `site.yaml`:
```yaml
sync:
  strategy: webhook
  webhook:
    path: /api/webhook
    branch_filter: main
    allowed_events:
      - push
```

Set the webhook secret:
```bash
export BLOGFLOW_WEBHOOK_SECRET="your-secret-here-min-32-chars!!!"
```

### 2. Add secrets to your content repository
In your content repo's GitHub Settings → Secrets:
- `BLOGFLOW_WEBHOOK_URL`: Your BlogFlow webhook URL (e.g., `https://blog.example.com/api/webhook`)
- `BLOGFLOW_WEBHOOK_SECRET`: Same secret as configured on the server

### 3. Copy the workflow
Copy `.github/workflows/content-deploy.yml` from the BlogFlow repo to your content repository's `.github/workflows/` directory.

### 4. Push content
Any push to the `main` branch will automatically trigger a content refresh on your BlogFlow instance.

## How it works
1. You push a content change to your content repo
2. GitHub Actions computes an HMAC-SHA256 signature using your shared secret
3. The workflow sends a POST to your BlogFlow webhook endpoint
4. BlogFlow verifies the signature, checks the branch, and triggers a content reload
5. Your blog updates within seconds

## Troubleshooting
- **401 Unauthorized**: Secret mismatch between the workflow and BlogFlow server
- **404 Not Found**: Wrong webhook URL or webhook path not configured
- **429 Too Many Requests**: Rate limit exceeded; wait and retry
- **500 Internal Server Error**: BlogFlow failed to reload content; check server logs
