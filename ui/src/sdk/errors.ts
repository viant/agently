export class HttpError extends Error {
  status: number;
  statusText: string;
  body: string;

  constructor(status: number, statusText: string, body: string) {
    super(`request failed: ${statusText} (${status}): ${body}`);
    this.status = status;
    this.statusText = statusText;
    this.body = body;
  }
}
