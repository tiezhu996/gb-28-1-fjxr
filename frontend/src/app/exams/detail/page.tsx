'use client';
import { Suspense, useCallback, useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useAuth } from '@/hooks/useAuth';
import { examApi } from '@/api/exam';
import { recordApi } from '@/api/record';
import { timeExtensionApi } from '@/api/timeExtension';
import { useExamStore } from '@/stores/examStore';
import { questionApi } from '@/api/question';
import { DataTable, type Column } from '@/components/DataTable';
import { Pagination } from '@/components/Pagination';
import { StatusBadge } from '@/components/StatusBadge';
import { examStatusColor, examStatusText, formatDateTime, questionTypeText, timeExtensionStatusColor, timeExtensionStatusText } from '@/utils/format';
import { EXAM_STATUS, TIME_EXTENSION_MAX_MINUTES, TIME_EXTENSION_MIN_MINUTES } from '@/constants';
import type { Exam, ExamRecord, Question, TimeExtension } from '@/types';

function ExamDetail() {
  const router = useRouter();
  const params = useSearchParams();
  const id = params.get('id') ?? '';
  const { isTeacher, isAdmin, isStudent } = useAuth();
  const canManage = isTeacher || isAdmin;
  const { publish, close, remove } = useExamStore();

  const [exam, setExam] = useState<Exam | null>(null);
  const [questions, setQuestions] = useState<Record<string, Question>>({});
  const [records, setRecords] = useState<ExamRecord[]>([]);
  const [recordsTotal, setRecordsTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [starting, setStarting] = useState(false);

  // 补时登记表单状态
  const [studentIdInput, setStudentIdInput] = useState('');
  const [extraMinutes, setExtraMinutes] = useState(10);
  const [reason, setReason] = useState('');
  const [granting, setGranting] = useState(false);
  const [revokingId, setRevokingId] = useState<string>('');

  const load = useCallback(async () => {
    if (!id) return;
    const e = await examApi.get(id);
    setExam(e);
    const qids = e.questions.map((q) => q.question_id);
    const qmap: Record<string, Question> = {};
    for (const qid of qids) {
      try {
        const q = await questionApi.get(qid);
        qmap[qid] = q;
      } catch {
        // 忽略已删除题目
      }
    }
    setQuestions(qmap);
    if (canManage) {
      const res = await recordApi.listByExam(id, { page, page_size: 10 });
      setRecords(res.list);
      setRecordsTotal(res.total);
    }
  }, [id, canManage, page]);

  useEffect(() => {
    load();
  }, [load]);

  const onStart = async () => {
    if (!id) return;
    setStarting(true);
    try {
      const rec = await recordApi.start(id);
      router.push(`/exam-take?recordId=${rec.id}`);
    } catch (err) {
      alert((err as Error).message);
    } finally {
      setStarting(false);
    }
  };

  // 教师登记补时：交卷前提交分钟数与原因（一场考试一名考生仅一条有效记录）
  const onGrant = async () => {
    if (!id) return;
    const sid = studentIdInput.trim();
    if (!sid) {
      alert('请填写考生 ID');
      return;
    }
    if (!reason.trim()) {
      alert('请填写补时原因');
      return;
    }
    if (extraMinutes < TIME_EXTENSION_MIN_MINUTES || extraMinutes > TIME_EXTENSION_MAX_MINUTES) {
      alert(`补时分钟数必须在 ${TIME_EXTENSION_MIN_MINUTES}~${TIME_EXTENSION_MAX_MINUTES} 之间`);
      return;
    }
    setGranting(true);
    try {
      await timeExtensionApi.grant(id, { student_id: sid, extra_minutes: extraMinutes, reason: reason.trim() });
      alert('补时登记成功');
      setStudentIdInput('');
      setReason('');
      setExtraMinutes(10);
      await load();
    } catch (err) {
      alert((err as Error).message);
    } finally {
      setGranting(false);
    }
  };

  // 开考前撤销补时记录（开考后冻结，按钮不展示）
  const onRevoke = async (te: TimeExtension) => {
    if (!confirm(`确认撤销 ${te.student_name} 的 ${te.extra_minutes} 分钟补时？`)) return;
    setRevokingId(te.id);
    try {
      await timeExtensionApi.revoke(te.id);
      await load();
    } catch (err) {
      alert((err as Error).message);
    } finally {
      setRevokingId('');
    }
  };

  if (!exam) return <div className="p-10 text-center text-gray-400">加载中…</div>;

  const extensions = exam.time_extensions ?? [];
  // 学生视角：仅本人有效补时（后端已按角色过滤）
  const myExtension = extensions.find((te) => te.status === 'active');
  // 开考前可撤销：统一开考时间未到
  const beforeStart = new Date(exam.start_at).getTime() > Date.now();

  const recordColumns: Column<ExamRecord>[] = [
    { key: 'student_name', title: '学生', render: (r) => <span>{r.student_name}</span> },
    { key: 'status', title: '状态', render: (r) => <StatusBadge text={r.status === 'graded' ? '已批改' : r.status === 'submitted' ? '已提交' : '答题中'} color={r.status === 'graded' ? 'green' : r.status === 'submitted' ? 'blue' : 'orange'} /> },
    { key: 'extra_minutes', title: '补时', render: (r) => (
        r.extra_minutes > 0 ? (
          <span className="text-xs font-medium text-emerald-600" title={r.deadline_at ? `个人截止 ${formatDateTime(r.deadline_at)}` : ''}>
            +{r.extra_minutes}分钟
          </span>
        ) : <span className="text-xs text-gray-300">-</span>
      ) },
    { key: 'objective_score', title: '客观题分', render: (r) => <span>{r.objective_score}</span> },
    { key: 'final_score', title: '最终分', render: (r) => <span className="font-medium">{r.final_score || '-'}</span> },
    { key: 'cheat_count', title: '切屏次数', render: (r) => <span className={r.cheat_count > 0 ? 'text-red-600' : ''}>{r.cheat_count}</span> },
    { key: 'started_at', title: '开始时间', render: (r) => <span className="text-xs">{formatDateTime(r.started_at)}</span> },
    { key: 'actions', title: '操作', render: (r) => (
        <button onClick={() => router.push(`/records/review?recordId=${r.id}`)} className="text-brand-600 hover:underline">查看/批改</button>
      ) },
  ];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-gray-800">{exam.title}</h1>
          <p className="mt-1 text-sm text-gray-500">
            {exam.subject} · 总分 {exam.total_score} · 及格 {exam.pass_score} · 时长 {exam.duration_min} 分钟
          </p>
          <p className="mt-1 text-xs text-gray-400">
            {formatDateTime(exam.start_at)} ~ {formatDateTime(exam.end_at)}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <StatusBadge text={examStatusText(exam.status)} color={examStatusColor(exam.status)} />
          {isStudent && exam.status === EXAM_STATUS.PUBLISHED && (
            <button onClick={onStart} disabled={starting}
              className="rounded-lg bg-green-600 px-4 py-2 text-sm text-white hover:bg-green-700 disabled:opacity-60">
              {starting ? '进入中…' : '开始考试'}
            </button>
          )}
          {canManage && (
            <>
              {exam.status === EXAM_STATUS.DRAFT && (
                <button onClick={async () => { await publish(id); load(); }}
                  className="rounded-lg bg-brand-600 px-4 py-2 text-sm text-white hover:bg-brand-700">发布</button>
              )}
              {exam.status !== EXAM_STATUS.CLOSED && (
                <button onClick={async () => { await close(id); load(); }}
                  className="rounded-lg border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50">关闭</button>
              )}
              <button onClick={async () => { if (confirm('确认删除？')) { await remove(id); router.push('/exams'); } }}
                className="rounded-lg border border-red-300 px-4 py-2 text-sm text-red-600 hover:bg-red-50">删除</button>
              <button onClick={() => router.push(`/reports?examId=${id}`)}
                className="rounded-lg border border-brand-600 px-4 py-2 text-sm text-brand-600 hover:bg-brand-50">成绩分析</button>
            </>
          )}
        </div>
      </div>

      {isStudent && myExtension && (
        <section className="rounded-xl border border-emerald-200 bg-emerald-50 px-5 py-3 text-sm text-emerald-800">
          本场考试已为您补时 <span className="font-semibold">{myExtension.extra_minutes} 分钟</span>
          {myExtension.deadline_at && <>，个人截止时间 <span className="font-semibold">{formatDateTime(myExtension.deadline_at)}</span></>}
          。开考与收卷按个人截止时间执行，试卷总分与标准答案不变。
        </section>
      )}

      <section className="rounded-xl border border-gray-200 bg-white p-5">
        <h2 className="font-semibold text-gray-800">试卷题目（{exam.questions.length} 题）</h2>
        <div className="mt-3 space-y-2">
          {exam.questions.map((eq) => {
            const q = questions[eq.question_id];
            return (
              <div key={eq.question_id} className="flex items-center gap-3 rounded-lg bg-gray-50 px-3 py-2 text-sm">
                <span className="w-6 text-center font-medium text-gray-400">{eq.order}</span>
                <span className="flex-1 truncate">{q ? q.content : eq.question_id}</span>
                <span className="text-xs text-gray-400">{q ? questionTypeText(q.type) : ''} · {eq.score}分</span>
              </div>
            );
          })}
        </div>
      </section>

      {canManage && (
        <section className="rounded-xl border border-gray-200 bg-white p-5">
          <h2 className="font-semibold text-gray-800">个别考生补时</h2>
          <p className="mt-1 text-xs text-gray-400">
            交卷前登记，补时 {TIME_EXTENSION_MIN_MINUTES}~{TIME_EXTENSION_MAX_MINUTES} 分钟，一名考生一场考试仅一条有效记录；开考前可撤销，开考后冻结。试卷总分与标准答案不变。
          </p>
          <div className="mt-3 grid gap-2 sm:grid-cols-[1fr_120px_2fr_auto]">
            <input
              value={studentIdInput}
              onChange={(e) => setStudentIdInput(e.target.value)}
              placeholder="考生 ID（可在考生记录中复制）"
              className="rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
            <input
              type="number"
              min={TIME_EXTENSION_MIN_MINUTES}
              max={TIME_EXTENSION_MAX_MINUTES}
              value={extraMinutes}
              onChange={(e) => setExtraMinutes(Number(e.target.value))}
              placeholder="补时分钟"
              className="rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="补时原因（必填）"
              className="rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
            <button onClick={onGrant} disabled={granting}
              className="rounded-lg bg-brand-600 px-4 py-2 text-sm text-white hover:bg-brand-700 disabled:opacity-60">
              {granting ? '登记中…' : '登记补时'}
            </button>
          </div>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-100 text-left text-xs text-gray-400">
                  <th className="py-2 pr-3 font-medium">考生</th>
                  <th className="py-2 pr-3 font-medium">补时分钟</th>
                  <th className="py-2 pr-3 font-medium">个人截止时间</th>
                  <th className="py-2 pr-3 font-medium">原因</th>
                  <th className="py-2 pr-3 font-medium">状态</th>
                  <th className="py-2 pr-3 font-medium">登记人</th>
                  <th className="py-2 font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {(exam.time_extensions ?? []).length === 0 && (
                  <tr><td colSpan={7} className="py-4 text-center text-xs text-gray-300">暂无补时记录</td></tr>
                )}
                {(exam.time_extensions ?? []).map((te) => (
                  <tr key={te.id} className="border-b border-gray-50">
                    <td className="py-2 pr-3">{te.student_name}<span className="ml-1 text-xs text-gray-300">{te.student_id}</span></td>
                    <td className="py-2 pr-3">+{te.extra_minutes} 分钟</td>
                    <td className="py-2 pr-3 text-xs">{te.deadline_at ? formatDateTime(te.deadline_at) : '-'}</td>
                    <td className="max-w-[200px] truncate py-2 pr-3 text-xs" title={te.reason}>{te.reason}</td>
                    <td className="py-2 pr-3"><StatusBadge text={timeExtensionStatusText(te.status)} color={timeExtensionStatusColor(te.status)} /></td>
                    <td className="py-2 pr-3 text-xs">{te.granted_by_name}</td>
                    <td className="py-2">
                      {te.status === 'active' && beforeStart ? (
                        <button onClick={() => onRevoke(te)} disabled={revokingId === te.id}
                          className="text-red-600 hover:underline disabled:opacity-50">
                          {revokingId === te.id ? '撤销中…' : '撤销'}
                        </button>
                      ) : (
                        <span className="text-xs text-gray-300">{te.status === 'active' ? '已开考冻结' : '-'}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {canManage && (
        <section className="rounded-xl border border-gray-200 bg-white p-5">
          <h2 className="font-semibold text-gray-800">考生记录</h2>
          <div className="mt-3">
            <DataTable columns={recordColumns} rows={records} emptyTitle="暂无考生作答" />
            <Pagination page={page} pageSize={10} total={recordsTotal} onChange={setPage} />
          </div>
        </section>
      )}
    </div>
  );
}

export default function ExamDetailPage() {
  return (
    <Suspense fallback={<div className="p-10 text-center text-gray-400">加载中…</div>}>
      <ExamDetail />
    </Suspense>
  );
}
