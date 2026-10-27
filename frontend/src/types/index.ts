export type Challenge = {
  id: string;
  title: string;
  description: string;
  category: "docker" | "kubernetes";
  difficulty: "easy" | "medium" | "hard";
  instructions: string;
  hints: string[];
  points: number;
  is_published: boolean;
  verification_json?: string;
  expected_answer?: string;
  created_at: string;
};

export type Attempt = {
  id: string;
  user_id: string;
  challenge_id: string;
  status: "in_progress" | "passed" | "failed" | "expired";
  environment_id?: string;
  environment_type?: string;
  credits_consumed: number;
  started_at: string;
  ended_at?: string;
  solved_at?: string;
  solve_time_seconds?: number;
};

export type Submission = {
  id: string;
  attempt_id: string;
  answer: string;
  passed: boolean;
  feedback: string;
  created_at: string;
};

export type User = {
  id: string;
  google_sub: string;
  email: string;
  name: string;
  picture?: string;
  role: string;
  credits: number;
  created_at: string;
};

export type Category = "docker" | "kubernetes";
export type Difficulty = "easy" | "medium" | "hard";

export const DIFFICULTY_LABELS: Record<Difficulty, string> = {
  easy: "Easy",
  medium: "Medium",
  hard: "Hard",
};

export const CATEGORY_LABELS: Record<Category, string> = {
  docker: "Docker",
  kubernetes: "Kubernetes",
};
