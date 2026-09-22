import { request } from '@/utils/request';
import type { TimeExtension } from '@/types';

export interface GrantTimeExtensionInput {
  student_id: string;
  extra_minutes: number; // 5~60
  reason: string;
}

export const timeExtensionApi = {
  // 教师为个别考生登记补时（交卷前：分钟数 + 原因）
  grant(examId: string, data: GrantTimeExtensionInput) {
    return request<TimeExtension>(`/exams/${examId}/time-extensions`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },
  // 开考前撤销补时记录（开考后冻结）
  revoke(extensionId: string) {
    return request<TimeExtension>(`/exam-time-extensions/${extensionId}/revoke`, { method: 'POST' });
  },
};
