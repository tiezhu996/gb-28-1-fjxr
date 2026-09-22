'use client';
// 个别考生补时管理（教师考试详情内嵌）：登记补时分钟与原因、开考前撤销、展示补时与个人截止时间。
import { useCallback, useEffect, useState } from 'react';
import { extensionApi, type GrantExtensionInput, type StudentOption } from '@/api/extension';
import { Modal } from '@/components/Modal';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { StatusBadge } from '@/components/StatusBadge';
import { extensionStatusColor, extensionStatusText, formatDateTime } from '@/utils/format';
import { EXTENSION_MAX_MINUTES, EXTENSION_MIN_MINUTES } from '@/constants';
import type { Exam, ExamExtension } from '@/types';

export function ExtensionManager({ exam, reloadKey }: { exam: Exam; reloadKey?: number }) {
  const [extensions, setExtensions] = useState<ExamExtension[]>([]);
  const [loading, setLoading] = useState(false);
  const [grantOpen, setGrantOpen] = useState(false);
  const [students, setStudents] = useState<StudentOption[]>([]);
  const [keyword, setKeyword] = useState('');
  const [form, setForm] = useState<GrantExtensionInput>({ student_id: '', extra_minutes: EXTENSION_MIN_MINUTES, reason: '' });
  const [saving, setSaving] = useState(false);
  const [revokeTarget, setRevokeTarget] = useState<ExamExtension | null>(null);
  const [revoking, setRevoking] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const list = await extensionApi.listByExam(exam.id);
      setExtensions(list);
    } catch (err) {
      alert((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [exam.id]);

  useEffect(() => {
    load();
  }, [load, reloadKey]);

  // 是否已开考：开考后补时冻结，不可撤销（也不允许新增登记按钮）。
  const now = Date.now();
  const started = new Date(exam.start_at).getTime() <= now;
  const ended = new Date(exam.end_at).getTime() <= now;
  const canGrant = !ended; // 交卷前可登记
  const canRevoke = !started; // 开考前可撤销

  const fetchStudents = useCallback(async (kw = '') => {
    try {
      setStudents(await extensionApi.listStudents(kw || undefined));
    } catch (err) {
      alert((err as Error).message);
    }
  }, []);

  const openGrant = async () => {
    setForm({ student_id: '', extra_minutes: EXTENSION_MIN_MINUTES, reason: '' });
    setKeyword('');
    setGrantOpen(true);
    fetchStudents('');
  };

  const onGrant = async () => {
    if (!form.student_id) {
      alert('请选择考生');
      return;
    }
    if (form.extra_minutes < EXTENSION_MIN_MINUTES || form.extra_minutes > EXTENSION_MAX_MINUTES) {
      alert(`补时分钟数须在 ${EXTENSION_MIN_MINUTES}~${EXTENSION_MAX_MINUTES} 分钟之间`);
      return;
    }
    if (!form.reason.trim()) {
      alert('请填写补时原因');
      return;
    }
    setSaving(true);
    try {
      await extensionApi.grant(exam.id, { ...form, reason: form.reason.trim() });
      setGrantOpen(false);
      await load();
    } catch (err) {
      alert((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const onRevoke = async () => {
    if (!revokeTarget) return;
    setRevoking(true);
    try {
      await extensionApi.revoke(revokeTarget.id);
      setRevokeTarget(null);
      await load();
    } catch (err) {
      alert((err as Error).message);
    } finally {
      setRevoking(false);
    }
  };

  // 个人截止时间：未开考 = 原结束时间 + 补时；已开考则交由记录快照展示。
  const personalEnd = (ext: ExamExtension) => {
    const base = new Date(exam.end_at).getTime() + ext.extra_minutes * 60 * 1000;
    return formatDateTime(new Date(base).toISOString());
  };

  return (
    <section className="rounded-xl border border-gray-200 bg-white p-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="font-semibold text-gray-800">个别考生延时（补时）</h2>
          <p className="mt-1 text-xs text-gray-400">
            仅对所选考生长生效，不影响其他考生；试卷总分与标准答案不变。补时限 {EXTENSION_MIN_MINUTES}~{EXTENSION_MAX_MINUTES} 分钟，
            {started ? '已开考，补时记录冻结不可撤销' : '开考前可撤销'}。
          </p>
        </div>
        {canGrant && (
          <button onClick={openGrant}
            className="rounded-lg bg-brand-600 px-4 py-2 text-sm text-white hover:bg-brand-700">
            登记补时
          </button>
        )}
      </div>

      <div className="mt-3">
        {loading ? (
          <p className="py-4 text-center text-sm text-gray-400">加载中…</p>
        ) : extensions.length === 0 ? (
          <p className="py-4 text-center text-sm text-gray-400">暂无补时记录</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200 text-left text-xs text-gray-400">
                <th className="py-2 pr-2">考生</th>
                <th className="py-2 pr-2">补时</th>
                <th className="py-2 pr-2">个人截止时间</th>
                <th className="py-2 pr-2">原因</th>
                <th className="py-2 pr-2">状态</th>
                <th className="py-2 pr-2">登记时间</th>
                <th className="py-2">操作</th>
              </tr>
            </thead>
            <tbody>
              {extensions.map((ext) => (
                <tr key={ext.id} className="border-b border-gray-100">
                  <td className="py-2 pr-2">{ext.student_name}</td>
                  <td className="py-2 pr-2 font-medium text-brand-700">+{ext.extra_minutes} 分钟</td>
                  <td className="py-2 pr-2 text-xs">{personalEnd(ext)}</td>
                  <td className="max-w-[180px] truncate py-2 pr-2 text-xs text-gray-500" title={ext.reason}>{ext.reason}</td>
                  <td className="py-2 pr-2">
                    <StatusBadge text={extensionStatusText(ext.status)} color={extensionStatusColor(ext.status)} />
                  </td>
                  <td className="py-2 pr-2 text-xs text-gray-400">{formatDateTime(ext.created_at)}</td>
                  <td className="py-2">
                    {ext.status === 'active' && canRevoke ? (
                      <button onClick={() => setRevokeTarget(ext)} className="text-red-600 hover:underline">撤销</button>
                    ) : ext.status === 'active' ? (
                      <span className="text-xs text-gray-400">已冻结</span>
                    ) : (
                      <span className="text-xs text-gray-400">-</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <Modal open={grantOpen} title="个别考生延时登记" onClose={() => setGrantOpen(false)}>
        <div className="space-y-4">
          <div>
            <label className="mb-1 block text-sm text-gray-600">考生（仅正常状态学生）</label>
            <input
              type="text"
              value={keyword}
              onChange={(e) => {
                setKeyword(e.target.value);
                fetchStudents(e.target.value);
              }}
              placeholder="按姓名或邮箱搜索"
              className="mb-2 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
            <select
              value={form.student_id}
              onChange={(e) => setForm((f) => ({ ...f, student_id: e.target.value }))}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            >
              <option value="">请选择考生</option>
              {students.map((u) => (
                <option key={u.id} value={u.id}>{u.name}（{u.email}）</option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-sm text-gray-600">补时分钟数（{EXTENSION_MIN_MINUTES}~{EXTENSION_MAX_MINUTES} 分钟）</label>
            <input
              type="number"
              min={EXTENSION_MIN_MINUTES}
              max={EXTENSION_MAX_MINUTES}
              value={form.extra_minutes}
              onChange={(e) => setForm((f) => ({ ...f, extra_minutes: Number(e.target.value) }))}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm text-gray-600">补时原因（交卷前登记）</label>
            <textarea
              rows={3}
              maxLength={500}
              value={form.reason}
              onChange={(e) => setForm((f) => ({ ...f, reason: e.target.value }))}
              placeholder="如：设备故障、网络中断、身体不适等"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
          </div>
          <div className="flex justify-end gap-3">
            <button onClick={() => setGrantOpen(false)}
              className="rounded-lg border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50">取消</button>
            <button onClick={onGrant} disabled={saving}
              className="rounded-lg bg-brand-600 px-4 py-2 text-sm text-white hover:bg-brand-700 disabled:opacity-60">
              {saving ? '提交中…' : '确认登记'}
            </button>
          </div>
        </div>
      </Modal>

      <ConfirmDialog
        open={!!revokeTarget}
        title="撤销补时"
        message={`确认撤销考生「${revokeTarget?.student_name}」+${revokeTarget?.extra_minutes} 分钟的补时？开考后将无法撤销。`}
        confirmText="确认撤销"
        loading={revoking}
        onConfirm={onRevoke}
        onCancel={() => setRevokeTarget(null)}
      />
    </section>
  );
}
