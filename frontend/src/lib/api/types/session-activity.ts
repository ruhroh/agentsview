export interface SessionActivityBucket {
  start_time: string;
  end_time: string;
  user_count: number;
  assistant_count: number;
  first_ordinal: number | null;
}

export interface SessionActivityResponse {
  buckets: SessionActivityBucket[];
  interval_seconds: number;
  total_messages: number;
}
