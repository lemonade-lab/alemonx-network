import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { fetchPrivilegeAudit, fetchPrivilegeStatus, preflightPrivilege, runActionAndPoll, type ActionResult, type PrivilegePreflight, type PrivilegeStatus } from './api'

type View = 'overview' | 'connections' | 'services' | 'security' | 'diagnostics'
type Capability = { id: string; label: string; available: boolean; elevated: boolean; reason?: string }
type CapabilitySnapshot = { platform: string; capabilities: Capability[] }
type InterfaceSnapshot = { name: string; up: boolean; mtu: number; mac?: string; addresses: string[] }
type Snapshot = { platform: string; capturedAt: string; interfaces: InterfaceSnapshot[]; defaultRoute: string; traffic: string; fingerprint: string }
type Plan = { id: string; operation: string; params: Record<string, string>; risk: string; impact: string; verification: string[]; expiresAt: string }
type Audit = { id: string; operation: string; params: Record<string, string>; output: string; undoOperation?: string; createdAt: string }
type Diagnosis = { target: string; steps: { id: string; label: string; status: string; detail: string; latencyMs?: number }[] }
type PrivilegeRequest = { action: 'apply-plan' | 'undo-last'; planID?: string; preflight: PrivilegePreflight; error?: string }

const labels: Record<string, string> = {
  'iface-up': '启用接口', 'iface-down': '停用接口', 'ip-add': '添加 IP 地址', 'ip-remove': '移除 IP 地址',
  'dns-set': '修改 DNS', 'mtu-set': '修改 MTU', 'route-add': '添加路由', 'route-remove': '移除路由',
  'forward-add': '发布转发服务', 'forward-remove': '停止转发服务', 'open-port': '开放防火墙端口', 'close-port': '关闭防火墙端口',
  'bond-create': '创建 Bond', 'bridge-create': '创建网桥', 'vlan-create': '创建 VLAN'
}

function Card({ title, detail, children }: { title: string; detail?: string; children: ReactNode }) {
  return <section className="rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 shadow-[var(--theme-shadow-soft)]"><div className="mb-3"><h2 className="m-0 text-sm font-semibold text-[var(--theme-text-strong)]">{title}</h2>{detail && <p className="m-0 mt-1 text-xs text-[var(--theme-text-muted)]">{detail}</p>}</div>{children}</section>
}

function Field({ label, name, defaultValue, type = 'text', hint }: { label: string; name: string; defaultValue?: string; type?: string; hint?: string }) {
  return <label className="grid gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">{label}<input className="min-h-9 rounded-control border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 text-sm font-normal text-[var(--theme-text-primary)]" name={name} type={type} defaultValue={defaultValue}/>{hint && <span className="font-normal text-[11px] text-[var(--theme-text-muted)]">{hint}</span>}</label>
}

function Status({ ok, children }: { ok: boolean; children: ReactNode }) {
  return <span className={ok ? 'text-[var(--theme-success-text)]' : 'text-[var(--theme-danger-text)]'}>{ok ? '● ' : '● '}{children}</span>
}

export default function App() {
  const [view, setView] = useState<View>('overview')
  const [capabilities, setCapabilities] = useState<CapabilitySnapshot>()
  const [snapshot, setSnapshot] = useState<Snapshot>()
  const [audit, setAudit] = useState<Audit[]>([])
  const [diagnosis, setDiagnosis] = useState<Diagnosis>()
  const [message, setMessage] = useState<ActionResult>()
  const [busy, setBusy] = useState(false)
  const [plan, setPlan] = useState<Plan>()
  const [privilege, setPrivilege] = useState<PrivilegeStatus>()
	const [privilegeRequest, setPrivilegeRequest] = useState<PrivilegeRequest>()

  const refresh = async () => {
    setBusy(true)
    try {
      const [capabilityResult, snapshotResult, auditResult, privilegeResult] = await Promise.all([
        runActionAndPoll<CapabilitySnapshot>('capabilities', {}), runActionAndPoll<Snapshot>('snapshot', {}), fetchPrivilegeAudit(), fetchPrivilegeStatus()
      ])
      setCapabilities(capabilityResult.data); setSnapshot(snapshotResult.data); setAudit((auditResult.items ?? []) as Audit[]); setPrivilege(privilegeResult)
      setMessage(snapshotResult)
    } finally { setBusy(false) }
  }
  useEffect(() => { void refresh() }, [])

  const available = (id: string) => capabilities?.capabilities.find(item => item.id === id)
  const submitPlan = async (operation: string, form: HTMLFormElement) => {
    const params = Object.fromEntries(new FormData(form).entries()) as Record<string, string>
    setBusy(true)
    try {
      const result = await runActionAndPoll<Plan>('plan', { operation, ...params })
      if (result.error) setMessage(result); else setPlan(result.data)
    } finally { setBusy(false) }
  }
  const requestPrivilege = async (action: 'apply-plan' | 'undo-last', planID?: string) => {
    setBusy(true)
    try {
      const preflight = await preflightPrivilege(action, planID)
			setPrivilegeRequest({ action, planID, preflight })
			setPlan(undefined)
    } catch (reason) { setMessage({ output: '', error: reason instanceof Error ? reason.message : String(reason) }) } finally { setBusy(false) }
  }
  const executePrivilege = async (password?: string) => {
		if (!privilegeRequest?.preflight.available || !privilegeRequest.preflight.intentId) return
		setBusy(true)
		try {
			const params: Record<string, string> = privilegeRequest.planID ? { planID: privilegeRequest.planID } : {}
			const result = await runActionAndPoll(privilegeRequest.action, params, true, privilegeRequest.preflight.intentId, password)
			setMessage(result)
			if (result.error && (result.error.includes('权限请求已') || result.error.includes('请先在工作台确认'))) {
				const preflight = await preflightPrivilege(privilegeRequest.action, privilegeRequest.planID)
				setPrivilegeRequest({ ...privilegeRequest, preflight, error: '权限请求已刷新，请重新确认后继续。' })
			} else if (result.error) setPrivilegeRequest({ ...privilegeRequest, error: result.error })
			else { setPrivilegeRequest(undefined); await refresh() }
		} catch (reason) { setPrivilegeRequest({ ...privilegeRequest, error: reason instanceof Error ? reason.message : String(reason) }) } finally { setBusy(false) }
	}
  const runDiagnostic = async (target: string) => { setBusy(true); try { const result = await runActionAndPoll<Diagnosis>('diagnose', { target }); setMessage(result); setDiagnosis(result.data) } finally { setBusy(false) } }
  const runRead = async (action: string) => { setBusy(true); try { const result = await runActionAndPoll(action, {}); setMessage(result) } finally { setBusy(false) } }

  const navigation: Array<[View, string, string]> = [['overview', '概览', '◉'], ['connections', '连接与接口', '⌁'], ['services', '服务与流量', '⇄'], ['security', '安全策略', '◈'], ['diagnostics', '诊断与历史', '◎']]
  return <main className="mx-auto grid max-w-[1120px] gap-4 p-4 lg:grid-cols-[190px_1fr]">
    <aside className="rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3 lg:min-h-[680px]"><div className="mb-5 px-2"><h1 className="m-0 text-base font-semibold text-[var(--theme-text-strong)]">网络治理台</h1><p className="m-0 mt-1 text-xs text-[var(--theme-text-muted)]">本机 · {capabilities?.platform ?? '正在识别系统'}</p></div><nav className="grid gap-1">{navigation.map(([id, label, icon]) => <button key={id} onClick={() => setView(id)} className={'flex min-h-9 items-center gap-2 rounded-control px-2.5 text-left text-xs font-semibold ' + (view === id ? 'bg-[var(--theme-surface-active)] text-[var(--theme-text-strong)]' : 'text-[var(--theme-text-muted)] hover:bg-[var(--theme-surface-hover)]')}><span>{icon}</span>{label}</button>)}</nav><button className="secondary-button mt-5 w-full" disabled={busy} onClick={() => void refresh()}>刷新快照</button></aside>
    <div className="grid content-start gap-4">
      {message && <div className={(message.error ? 'border-[var(--theme-danger)] bg-[var(--theme-danger-soft)] text-[var(--theme-danger-text)]' : 'border-[var(--theme-info)] bg-[var(--theme-info-soft)] text-[var(--theme-info-text)]') + ' rounded-panel border px-3 py-2 text-xs'}>{message.error || message.output}</div>}
      {!privilege?.network.enabled && <div className="rounded-panel border border-[var(--theme-warning)] bg-[var(--theme-warning-soft)] px-3 py-2 text-xs text-[var(--theme-warning-text)]">仅可预演：{privilege?.network.reason || privilege?.privilege.reason || '正在读取本机权限策略。'}</div>}
      {privilege && !privilege.audit.valid && <div className="rounded-panel border border-[var(--theme-danger)] bg-[var(--theme-danger-soft)] px-3 py-2 text-xs text-[var(--theme-danger-text)]">权限审计链校验失败：{privilege.audit.reason || '请停止系统变更并检查本机存储。'}</div>}
      {view === 'overview' && <Overview snapshot={snapshot} capabilities={capabilities} audit={audit} onRefresh={refresh} busy={busy}/>}
      {view === 'connections' && <Connections snapshot={snapshot} virtual={available('virtual')} busy={busy} onPlan={submitPlan}/>}
      {view === 'services' && <Services snapshot={snapshot} busy={busy} onPlan={submitPlan} onRead={runRead}/>}
      {view === 'security' && <Security firewall={available('firewall')} busy={busy} onPlan={submitPlan} onRead={runRead}/>}
		{view === 'diagnostics' && <Diagnostics diagnosis={diagnosis} audit={audit} busy={busy} onDiagnose={runDiagnostic} onUndo={() => requestPrivilege('undo-last')}/>}
    </div>
		{plan && <PlanModal plan={plan} busy={busy} onCancel={() => setPlan(undefined)} onApply={() => void requestPrivilege('apply-plan', plan.id)}/>}
		{privilegeRequest && <PrivilegeRequestModal request={privilegeRequest} busy={busy} onCancel={() => setPrivilegeRequest(undefined)} onExecute={password => void executePrivilege(password)} />}
  </main>
}

function Overview({ snapshot, capabilities, audit, onRefresh, busy }: { snapshot?: Snapshot; capabilities?: CapabilitySnapshot; audit: Audit[]; onRefresh: () => Promise<void>; busy: boolean }) {
  const risks = useMemo(() => capabilities?.capabilities.filter(item => !item.available) ?? [], [capabilities])
  return <><header><h2 className="m-0 text-lg font-semibold text-[var(--theme-text-strong)]">本机网络概览</h2><p className="m-0 mt-1 text-sm text-[var(--theme-text-muted)]">先观察，再预演，再变更。所有系统修改均要求单次授权。</p></header><div className="grid gap-3 md:grid-cols-3"><Card title="网络健康" detail={snapshot?.interfaces.some(item => item.up && item.addresses.length) ? '存在可用网络接口' : '未检测到已连接接口'}><Status ok={Boolean(snapshot?.interfaces.some(item => item.up && item.addresses.length))}>接口与地址</Status></Card><Card title="默认路由" detail={snapshot?.defaultRoute || '正在读取'}><span className="break-all text-xs text-[var(--theme-text-secondary)]">{snapshot?.platform}</span></Card><Card title="权限模型" detail="系统修改时按次请求管理员授权"><Status ok>观察无需特权</Status></Card></div><div className="grid gap-4 lg:grid-cols-2"><Card title="风险与能力" detail="不可用项不会显示为可执行操作"><div className="grid gap-2 text-xs">{risks.length ? risks.map(item => <div key={item.id} className="rounded-md bg-[var(--theme-warning-soft)] p-2 text-[var(--theme-warning-text)]">{item.label}：{item.reason}</div>) : <Status ok>当前平台的核心能力均可使用</Status>}</div></Card><Card title="近期变更" detail="本机保留最近 100 条记录"><div className="grid gap-2 text-xs">{audit.slice(0, 4).map(entry => <div key={entry.id} className="border-b border-[var(--theme-border-subtle)] pb-2"><strong>{labels[entry.operation] ?? entry.operation}</strong><span className="ml-2 text-[var(--theme-text-muted)]">{new Date(entry.createdAt).toLocaleString()}</span></div>)}{!audit.length && <span className="text-[var(--theme-text-muted)]">尚无变更记录。</span>}</div></Card></div><button className="primary-button w-fit" disabled={busy} onClick={() => void onRefresh()}>刷新网络快照</button></>
}

function Connections({ snapshot, virtual, busy, onPlan }: { snapshot?: Snapshot; virtual?: Capability; busy: boolean; onPlan: (operation: string, form: HTMLFormElement) => Promise<void> }) {
  return <><header><h2 className="m-0 text-lg font-semibold">连接与接口</h2><p className="m-0 mt-1 text-sm text-[var(--theme-text-muted)]">管理接口、地址、DNS、路由和 MTU；写操作会先生成计划。</p></header><Card title="接口状态"><div className="grid gap-2">{snapshot?.interfaces.map(item => <div key={item.name} className="grid gap-1 rounded-md border border-[var(--theme-border-subtle)] p-3 sm:grid-cols-[1fr_auto]"><div><Status ok={item.up}>{item.name} · MTU {item.mtu}</Status><div className="mt-1 text-xs text-[var(--theme-text-muted)]">{item.addresses.join(' · ') || '未分配地址'} {item.mac && `· ${item.mac}`}</div></div></div>) || <span className="text-xs text-[var(--theme-text-muted)]">正在加载接口。</span>}</div></Card><div className="grid gap-4 lg:grid-cols-2"><ChangeForm title="DNS 与 MTU" busy={busy} operation="dns-set" actionLabel="预演 DNS 修改" onPlan={onPlan}><Field label="接口" name="interface" hint="例如 en0、eth0"/><Field label="DNS 服务器" name="dns" hint="以空格分隔，例如 1.1.1.1 8.8.8.8"/></ChangeForm><ChangeForm title="静态 IP 地址" busy={busy} operation="ip-add" actionLabel="预演添加地址" onPlan={onPlan}><Field label="接口" name="interface"/><Field label="IP/CIDR" name="cidr" hint="例如 192.168.1.20/24"/></ChangeForm><ChangeForm title="路由" busy={busy} operation="route-add" actionLabel="预演添加路由" onPlan={onPlan}><Field label="目标 CIDR" name="cidr"/><Field label="网关" name="gateway"/><Field label="接口（可选）" name="interface"/></ChangeForm><Card title="Linux 虚拟网络" detail={virtual?.available ? '支持 Bond、网桥与 VLAN。创建前请确认成员接口没有业务连接。' : virtual?.reason ?? '正在识别'}><button className="secondary-button" disabled={!virtual?.available || busy}>高级能力将在受支持的 Linux 主机显示配置表单</button></Card></div></>
}

function Services({ snapshot, busy, onPlan, onRead }: { snapshot?: Snapshot; busy: boolean; onPlan: (operation: string, form: HTMLFormElement) => Promise<void>; onRead: (action: string) => Promise<void> }) {
  return <><header><h2 className="m-0 text-lg font-semibold">服务与流量</h2><p className="m-0 mt-1 text-sm text-[var(--theme-text-muted)]">将端口转发视为服务发布：先检查范围，再确认风险。</p></header><div className="grid gap-4 lg:grid-cols-2"><Card title="流量快照" detail="操作系统当前接口计数"><pre className="m-0 max-h-56 overflow-auto whitespace-pre-wrap text-xs text-[var(--theme-text-secondary)]">{snapshot?.traffic || '正在读取流量数据。'}</pre></Card><Card title="现有端口转发" detail="仅显示系统当前可枚举的规则"><button className="secondary-button" disabled={busy} onClick={() => void onRead('forward-list')}>刷新转发规则</button></Card></div><ChangeForm title="发布端口转发" busy={busy} operation="forward-add" actionLabel="预演服务发布" onPlan={onPlan}><Field label="本机端口" name="listenPort" type="number" defaultValue="17117"/><Field label="目标设备 IP" name="targetIP" hint="例如 192.168.1.100"/><Field label="目标端口" name="targetPort" type="number"/><Field label="协议" name="protocol" defaultValue="tcp" hint="当前 Windows 仅支持 TCP"/></ChangeForm></>
}

function Security({ firewall, busy, onPlan, onRead }: { firewall?: Capability; busy: boolean; onPlan: (operation: string, form: HTMLFormElement) => Promise<void>; onRead: (action: string) => Promise<void> }) {
  return <><header><h2 className="m-0 text-lg font-semibold">安全策略</h2><p className="m-0 mt-1 text-sm text-[var(--theme-text-muted)]">规则展示实际系统状态；仅管理由本插件创建的规则。</p></header>{firewall?.available ? <><Card title="有效防火墙状态" detail="建议先读取规则，再创建最小范围的入站访问"><button className="secondary-button" disabled={busy} onClick={() => void onRead('firewall-status')}>读取防火墙状态</button></Card><div className="grid gap-4 lg:grid-cols-2"><ChangeForm title="开放入站端口" busy={busy} operation="open-port" actionLabel="预演开放端口" onPlan={onPlan}><Field label="端口" name="port" type="number" defaultValue="17117"/><Field label="协议" name="protocol" defaultValue="tcp"/></ChangeForm><ChangeForm title="关闭入站端口" busy={busy} operation="close-port" actionLabel="预演关闭端口" onPlan={onPlan}><Field label="端口" name="port" type="number" defaultValue="17117"/><Field label="协议" name="protocol" defaultValue="tcp"/></ChangeForm></div></> : <Card title="防火墙策略不可用" detail={firewall?.reason ?? '正在识别平台能力'}><span className="text-xs text-[var(--theme-text-muted)]">请使用系统原生安全设置；本工具不会在不安全的情况下伪造端口规则支持。</span></Card>}</>
}

function Diagnostics({ diagnosis, audit, busy, onDiagnose, onUndo }: { diagnosis?: Diagnosis; audit: Audit[]; busy: boolean; onDiagnose: (target: string) => Promise<void>; onUndo: () => Promise<void> }) {
  return <><header><h2 className="m-0 text-lg font-semibold">诊断与变更历史</h2><p className="m-0 mt-1 text-sm text-[var(--theme-text-muted)]">按 DNS、TCP 与路由逐层确认故障位置；撤销仅针对支持逆操作的最近变更。</p></header><Card title="连通性诊断"><form className="flex flex-wrap gap-2" onSubmit={(event: FormEvent<HTMLFormElement>) => { event.preventDefault(); const target = String(new FormData(event.currentTarget).get('target') || ''); void onDiagnose(target) }}><input className="min-h-9 flex-1 rounded-control border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 text-sm" name="target" defaultValue="registry.npmjs.org"/><button className="primary-button" disabled={busy}>运行诊断</button></form>{diagnosis && <div className="mt-3 grid gap-2">{diagnosis.steps.map(step => <div key={step.id} className="rounded-md border border-[var(--theme-border-subtle)] p-2 text-xs"><Status ok={step.status === 'ok'}>{step.label}</Status><span className="ml-2 text-[var(--theme-text-muted)]">{step.detail}</span>{step.latencyMs ? <span className="ml-2 text-[var(--theme-text-faint)]">{step.latencyMs} ms</span> : null}</div>)}</div>}</Card><Card title="审计与撤销" detail="撤销会再次请求系统管理员权限"><div className="mb-3 grid gap-2 text-xs">{audit.slice(0, 8).map(entry => <div key={entry.id} className="border-b border-[var(--theme-border-subtle)] pb-2"><strong>{labels[entry.operation] ?? entry.operation}</strong><span className="ml-2 text-[var(--theme-text-muted)]">{entry.output}</span></div>)}{!audit.length && <span className="text-[var(--theme-text-muted)]">尚无记录。</span>}</div><button className="danger-button" disabled={busy || !audit.some(item => item.undoOperation)} onClick={() => void onUndo()}>撤销最近可恢复变更</button></Card></>
}

function ChangeForm({ title, operation, actionLabel, busy, onPlan, children }: { title: string; operation: string; actionLabel: string; busy: boolean; onPlan: (operation: string, form: HTMLFormElement) => Promise<void>; children: ReactNode }) {
  return <Card title={title} detail="不会立即修改系统。"><form className="grid gap-3" onSubmit={(event: FormEvent<HTMLFormElement>) => { event.preventDefault(); void onPlan(operation, event.currentTarget) }}><div className="grid gap-3 sm:grid-cols-2">{children}</div><button className="secondary-button w-fit" disabled={busy}>{actionLabel}</button></form></Card>
}

function PlanModal({ plan, busy, onCancel, onApply }: { plan: Plan; busy: boolean; onCancel: () => void; onApply: () => void }) {
  return <div className="fixed inset-0 z-50 grid place-items-center bg-[var(--theme-surface-overlay)] p-4"><section className="w-full max-w-xl rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-5 shadow-[var(--theme-shadow-pop)]"><h2 className="m-0 text-base font-semibold">确认变更计划</h2><p className="mt-1 text-sm text-[var(--theme-text-muted)]">{labels[plan.operation] ?? plan.operation} · 风险等级：<strong className={plan.risk === 'high' ? 'text-[var(--theme-danger-text)]' : 'text-[var(--theme-warning-text)]'}>{plan.risk === 'high' ? '高' : '中'}</strong></p><div className="mt-4 grid gap-3 rounded-md bg-[var(--theme-surface-raised)] p-3 text-xs"><div><strong>影响：</strong>{plan.impact}</div><div><strong>参数：</strong>{Object.entries(plan.params).map(([key, value]) => `${key}=${value}`).join('，')}</div><div><strong>验证：</strong>{plan.verification.join('、')}</div><div className="text-[var(--theme-text-muted)]">计划将在 {new Date(plan.expiresAt).toLocaleTimeString()} 失效；应用时会重新检查网络状态。</div></div><div className="mt-5 flex justify-end gap-2"><button className="secondary-button" disabled={busy} onClick={onCancel}>取消</button><button className="danger-button" disabled={busy} onClick={onApply}>确认并请求系统授权</button></div></section></div>
}

function PrivilegeRequestModal({ request, busy, onCancel, onExecute }: { request: PrivilegeRequest; busy: boolean; onCancel: () => void; onExecute: (password?: string) => void }) {
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const requiresPassword = request.preflight.authorization === 'password'
  const submit = () => {
    if (requiresPassword && !password) { setError('请输入当前系统账户的管理员密码。'); return }
    if (requiresPassword && password !== confirmation) { setError('两次输入的密码不一致。'); return }
    const value = password
    setPassword(''); setConfirmation(''); setError('')
    onExecute(requiresPassword ? value : undefined)
  }
  const nativeLabel = request.preflight.authorization === 'native-uac' ? '继续并调起 Windows UAC' : request.preflight.authorization === 'polkit' ? '继续并调起系统授权' : '确认授权'
  return <div className="fixed inset-0 z-[60] grid place-items-center bg-[var(--theme-surface-overlay)] p-4"><section className="grid w-full max-w-lg gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-5 shadow-[var(--theme-shadow-pop)]"><div><h2 className="m-0 text-base font-semibold">{request.preflight.title}</h2><p className="m-0 mt-2 text-sm leading-6 text-[var(--theme-text-muted)]">{request.preflight.description}</p></div>{!request.preflight.available ? <p className="m-0 rounded-md bg-[var(--theme-warning-soft)] p-3 text-xs leading-5 text-[var(--theme-warning-text)]">{request.preflight.reason || '当前无法请求系统权限。请在本机桌面工作台中重试。'}</p> : <>{requiresPassword && <><label className="grid gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">管理员密码<input autoFocus type="password" autoComplete="current-password" className="min-h-9 rounded-control border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 text-sm" value={password} onChange={event => { setPassword(event.target.value); setError('') }}/></label><label className="grid gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">确认管理员密码<input type="password" autoComplete="current-password" className="min-h-9 rounded-control border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 text-sm" value={confirmation} onChange={event => { setConfirmation(event.target.value); setError('') }}/></label></>}{(error || request.error) && <p className="m-0 rounded-md bg-[var(--theme-danger-soft)] p-2 text-xs text-[var(--theme-danger-text)]">{error || request.error}</p>}</>}<div className="flex justify-end gap-2"><button className="secondary-button" disabled={busy} onClick={() => { setPassword(''); setConfirmation(''); onCancel() }}>取消</button>{request.preflight.available && <button className="danger-button" disabled={busy} onClick={submit}>{requiresPassword ? '确认授权' : nativeLabel}</button>}</div></section></div>
}
