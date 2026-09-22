import { request, buildQuery } from '@/utils/request';
import type { ExamExtension, MyExtension } from '@/types';

export interface GrantExtensionInput {
  student_id: string;
  extra_minutes: number;
  reason: string;
}

export interface StudentOption {
  id: string;
  name: string;
  email: string;
  status: string;
}

// 个别考生补时接口（教师登记/撤销/查询，学生查询本人补时与个人截止时间）。
export const extensionApi = {
  listByExam(examId: string) {
    return request<ExamExtension[]>(`/exams/${examId}/extensions`);
  },
  grant(examId: string, data: GrantExtensionInput) {
    return request<ExamExtension>(`/exams/${examId}/extensions`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },
  revoke(extensionId: string) {
    return request<ExamExtension>(`/exams/extensions/${extensionId}`, { method: 'DELETE' });
  },
  mine(examId: string) {
    return request<MyExtension | null>(`/exams/${examId}/my-extension`);
  },
  listStudents(keyword?: string) {
    return request<StudentOption[]>(`/exam-extension-students${buildQuery({ keyword })}`);
  },
};
