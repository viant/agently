/**
 * Singleton FeedTracker from the SDK.
 * chatRuntime stores active feeds here via SSE events.
 * ToolFeedBar subscribes and renders the indicator above the composer.
 */
import { FeedTracker } from 'agently-core-ui-sdk';

export const feedTracker = new FeedTracker();

export function getActiveFeeds() {
  return feedTracker.feeds;
}

export function onFeedChange(fn) {
  return feedTracker.onChange(fn);
}
