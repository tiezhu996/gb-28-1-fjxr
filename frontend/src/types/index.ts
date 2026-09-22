// 与后端 dto 对应的前端类型定义。
export interface User {
  id: string;
  name: string;
  email: string;
  role: string;
  status: string;
  created_at: string;
}

export interface LoginResponse {
  token: string;
  user: User;
}

export interface PageResult<T> {
  list: T[];
  total: number;
  page: number;
  page_size: number;
  total_page: number;
}

export interface QuestionOption {
  key: string;
  text: string;
  score?: number;
}

export interface Question {
  id: string;
  type: string;
  subject: string;
  knowledge_points: string[];
  difficulty: string;
  content: string;
  options: QuestionOption[];
  answer: string;
  analysis: string;
  score: number;
  status: string;
  creator_id: string;
  created_at: string;
  updated_at: string;
}

export interface ExamQuestion {
  question_id: string;
  score: number;
  order: number;
}

export interface Exam {
  id: string;
  title: string;
  subject: string;
  description: string;
  total_score: number;
  pass_score: number;
  duration_min: number;
  start_at: string;
  end_at: string;
  status: string;
  shuffle_question: boolean;
  shuffle_option: boolean;
  questions: ExamQuestion[];
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface AttemptQuestion {
  question_id: string;
  type: string;
  subject: string;
  knowledge_points: string[];
  content: string;
  options: { key: string; text: string }[];
  score: number;
  correct_answer: string;
  user_answer: string;
  result: string;
  got_score: number;
  subjective_score?: number;
  comment?: string;
  marked?: boolean;
}

export interface ExamRecord {
  id: string;
  exam_id: string;
  exam_title: string;
  student_id: string;
  student_name: string;
  status: string;
  started_at: string;
  submitted_at?: string | null;
  objective_score: number;
  subjective_score: number;
  final_score: number;
  pass_score: number;
  cheat_count: number;
  auto_submitted: boolean;
  questions: AttemptQuestion[];
  // 个别考生补时快照
  extra_minutes: number;
  duration_min: number;
  deadline_at: string;
  personal_end_at: string;
  created_at: string;
}

// 个别考生补时记录（教师视角）。
export interface ExamExtension {
  id: string;
  exam_id: string;
  exam_title: string;
  student_id: string;
  student_name: string;
  extra_minutes: number;
  reason: string;
  status: string;
  status_text: string;
  granted_by: string;
  revoked_by?: string;
  revoked_at?: string | null;
  created_at: string;
}

// 学生查询本人补时记录响应（含个人截止时间）。
export interface MyExtension {
  exam_id: string;
  exam_title: string;
  extra_minutes: number;
  status: string;
  status_text: string;
  duration_min: number;
  personal_end_at: string;
  has_in_progress: boolean;
  started_at?: string;
  deadline_at?: string;
}

export interface WrongBook {
  id: string;
  student_id: string;
  question_id: string;
  exam_id: string;
  exam_record_id: string;
  subject: string;
  knowledge_points: string[];
  question_content: string;
  my_answer: string;
  correct_answer: string;
  analysis: string;
  note: string;
  status: string;
  created_at: string;
}

export interface AuditLog {
  id: string;
  user_id: string;
  username: string;
  role: string;
  module: string;
  action: string;
  method: string;
  path: string;
  status_code: number;
  request_id: string;
  client_ip: string;
  detail: string;
  created_at: string;
}

export interface ExamReport {
  exam_id: string;
  exam_title: string;
  total_students: number;
  average_score: number;
  max_score: number;
  min_score: number;
  pass_rate: number;
  score_bands: Record<string, number>;
  question_reports: {
    question_id: string;
    content: string;
    type: string;
    answer_count: number;
    correct_count: number;
    accuracy: number;
  }[];
}
