import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { fetchPrivilegeAudit, fetchPrivilegeStatus, fetchStatus, preflightPrivilege, runActionAndPoll, type ActionResult, type PrivilegePreflight, type PrivilegeStatus } from './api'

type View = 'governance' | 'ports' | 'firewall'
type Capability = { id: string; label: string; available: boolean; elevated: boolean; reason?: string }
type CapabilitySnapshot = { platform: string; capabilities: Capability[] }
type InterfaceSnapshot = { name: string; up: boolean; mtu: number; mac?: string; addresses: string[] }
type Snapshot = { platform: string; capturedAt: string; interfaces: InterfaceSnapshot[]; defaultRoute: string; traffic: string }
type FirewallStatus = { available: boolean; enabled?: boolean; backend?: string; detail?: string }
type Plan = { id: string; operation: string; params: Record<string, string>; risk: string; impact: string; verification: string[]; expiresAt: string }
type PrivilegeRequest = { action: 'apply-plan'; planID: string; preflight: PrivilegePreflight; error?: string }

const labels: Record<string, string> = {
  'dns-set': '修改 DNS', 'ip-add': '添加 IP 地址', 'route-add': '添加路由',
  'forward-add': '新建端口转发', 'open-port': '允许端口访问', 'close-port': '阻止端口访问', 'firewall-set': '切换防火墙'
}

function Icon({ name }: { name: View | 'refresh' | 'chevron' }) {
  const paths = {
    governance: <><path d="M4 17.5V12m5.3 5.5V6m5.4 11.5V9m5.3 8.5V3.5"/><path d="M2.5 20.5h19"/></>,
    ports: <><rect x="3.5" y="3.5" width="17" height="17" rx="3"/><path d="M8 8h8M8 12h8M8 16h4"/></>,
    firewall: <><path d="M12 2.8 19 6v5.2c0 4.4-2.9 7.8-7 9.4-4.1-1.6-7-5-7-9.4V6l7-3.2Z"/><path d="m8.7 11.8 2.1 2.1 4.5-4.5"/></>,
    refresh: <><path d="M20 11a8.1 8.1 0 0 0-14.8-3.4L3 10"/><path d="M3 4v6h6M4 13a8.1 8.1 0 0 0 14.8 3.4L21 14"/><path d="M21 20v-6h-6"/></>,
    chevron: <path d="m9 18 6-6-6-6"/>
  }
  return <svg aria-hidden="true" className="ui-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg>
}

function StatusDot({ ok }: { ok: boolean }) { return <span className={ok ? 'status-dot status-dot--good' : 'status-dot status-dot--quiet'} /> }

function SettingGroup({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return <section className="setting-group"><div className="setting-group__head"><h2>{title}</h2>{description && <p>{description}</p>}</div>{children}</section>
}

function Field({ label, name, defaultValue, type = 'text', hint }: { label: string; name: string; defaultValue?: string; type?: string; hint?: string }) {
  return <label className="form-field"><span>{label}</span><input name={name} type={type} defaultValue={defaultValue} />{hint && <small>{hint}</small>}</label>
}

function ActionForm({ title, description, operation, actionLabel, busy, children, onPlan, disabled }: { title: string; description: string; operation: string; actionLabel: string; busy: boolean; children: ReactNode; onPlan: (operation: string, form: HTMLFormElement) => Promise<void>; disabled?: boolean }) {
  return <div className="action-card"><div><h3>{title}</h3><p>{description}</p></div><form className="action-card__form" onSubmit={(event: FormEvent<HTMLFormElement>) => { event.preventDefault(); void onPlan(operation, event.currentTarget) }}><div className="form-grid">{children}</div><button className="secondary-button" disabled={busy || disabled}>{actionLabel}</button></form></div>
}

export default function App() {
  const [view, setView] = useState<View>('governance')
  const [capabilities, setCapabilities] = useState<CapabilitySnapshot>()
  const [snapshot, setSnapshot] = useState<Snapshot>()
  const [firewallStatus, setFirewallStatus] = useState<FirewallStatus>()
  const [message, setMessage] = useState<ActionResult>()
  const [busy, setBusy] = useState(false)
  const [plan, setPlan] = useState<Plan>()
  const [privilege, setPrivilege] = useState<PrivilegeStatus>()
  const [privilegeRequest, setPrivilegeRequest] = useState<PrivilegeRequest>()

  const refresh = async (notice = true) => {
    setBusy(true)
    try {
      const [capabilityResult, snapshotResult, firewallResult, privilegeResult] = await Promise.all([
        fetchStatus<CapabilitySnapshot>('capabilities'), fetchStatus<Snapshot>('snapshot'), fetchStatus<FirewallStatus>('firewall-status'), fetchPrivilegeStatus(), fetchPrivilegeAudit()
      ])
      setCapabilities(capabilityResult); setSnapshot(snapshotResult); setFirewallStatus(firewallResult); setPrivilege(privilegeResult)
      if (notice) setMessage({ output: '网络状态已更新。' })
    } catch (reason) { setMessage({ output: '', error: reason instanceof Error ? reason.message : String(reason) }) } finally { setBusy(false) }
  }
  useEffect(() => { void refresh(false) }, [])
  const available = (id: string) => capabilities?.capabilities.find(item => item.id === id)
  const submitPlan = async (operation: string, form: HTMLFormElement) => {
    const params = Object.fromEntries(new FormData(form).entries()) as Record<string, string>
    setBusy(true)
    try { const result = await runActionAndPoll<Plan>('plan', { operation, ...params }); if (result.error) setMessage(result); else setPlan(result.data) } finally { setBusy(false) }
  }
  const requestPrivilege = async (planID: string) => {
    setBusy(true)
    try { const preflight = await preflightPrivilege('apply-plan', planID); setPrivilegeRequest({ action: 'apply-plan', planID, preflight }); setPlan(undefined) }
    catch (reason) { setMessage({ output: '', error: reason instanceof Error ? reason.message : String(reason) }) } finally { setBusy(false) }
  }
  const executePrivilege = async (password?: string) => {
    if (!privilegeRequest?.preflight.available || !privilegeRequest.preflight.intentId) return
    setBusy(true)
    try {
      const result = await runActionAndPoll('apply-plan', { planID: privilegeRequest.planID }, true, privilegeRequest.preflight.intentId, password)
      if (result.error) setPrivilegeRequest({ ...privilegeRequest, error: result.error })
      else { setPrivilegeRequest(undefined); setMessage(result); await refresh(false) }
    } catch (reason) { setPrivilegeRequest({ ...privilegeRequest, error: reason instanceof Error ? reason.message : String(reason) }) } finally { setBusy(false) }
  }
  const navigation: Array<[View, string, string]> = [['governance', '服务器网络', '连接、地址与路由'], ['ports', '网络服务', '转发与流量'], ['firewall', '安全策略', '最小化入站暴露']]

  return <main className="settings-shell" data-app-settings-shell>
    <aside className="settings-sidebar" data-app-settings-sidebar aria-label="网络设置分类">
      <nav aria-label="网络设置" role="tablist" data-app-settings-nav>{navigation.map(([id, label]) => <button key={id} id={`network-settings-tab-${id}`} role="tab" aria-selected={view === id} aria-controls={`network-settings-panel-${id}`} onClick={() => setView(id)} className={view === id ? 'nav-item nav-item--active' : 'nav-item'}><Icon name={id}/><span>{label}</span></button>)}</nav>
      <div className="sidebar-footer"><button className="sidebar-action" title="刷新网络状态" disabled={busy} onClick={() => void refresh()}><Icon name="refresh"/>刷新网络状态</button><small><StatusDot ok={Boolean(snapshot?.interfaces.some(item => item.up && item.addresses.length))}/>{snapshot?.platform ?? '正在识别主机'}</small></div>
    </aside>
    <section className="settings-content" id={`network-settings-panel-${view}`} role="tabpanel" aria-labelledby={`network-settings-tab-${view}`}>
      <div className="settings-panel-content" data-app-settings-body>
      {message && <div role="status" className={message.error ? 'notice notice--error' : 'notice'}>{message.error || message.output}</div>}
      <div className="notice notice--warning">线上服务器操作提示：修改网络或防火墙前，请确认 SSH、远程桌面或其他管理通道仍有备用连接。</div>
      {privilege && !privilege.privilege.enabled && <div className="notice notice--warning">系统授权暂不可用：{privilege.privilege.reason || '请检查工作台系统权限设置。'}</div>}
      {view === 'governance' && <Governance snapshot={snapshot} busy={busy} onPlan={submitPlan}/>}
      {view === 'ports' && <Ports snapshot={snapshot} busy={busy} onPlan={submitPlan}/>}
      {view === 'firewall' && <Firewall firewall={available('firewall')} toggle={available('firewall-toggle')} status={firewallStatus} busy={busy} onPlan={submitPlan}/>}
      </div></section>
    {plan && <PlanModal plan={plan} busy={busy} onCancel={() => setPlan(undefined)} onApply={() => void requestPrivilege(plan.id)}/>}
    {privilegeRequest && <PrivilegeRequestModal request={privilegeRequest} busy={busy} onCancel={() => setPrivilegeRequest(undefined)} onExecute={password => void executePrivilege(password)}/>}
  </main>
}

function Governance({ snapshot, busy, onPlan }: { snapshot?: Snapshot; busy: boolean; onPlan: (operation: string, form: HTMLFormElement) => Promise<void> }) {
  const online = Boolean(snapshot?.interfaces.some(item => item.up && item.addresses.length))
  return <div className="settings-stack"><div className="summary-card"><div className="summary-card__icon"><Icon name="governance"/></div><div><p>线上服务器连接状态</p><h2><StatusDot ok={online}/>{online ? '网络已连接' : '等待网络连接'}</h2><span>{snapshot?.defaultRoute ? `默认路由：${snapshot.defaultRoute}` : '正在读取网络信息…'}</span></div></div>
    <SettingGroup title="网络接口" description="查看本机接口和当前分配的地址。"><div className="interface-list">{snapshot?.interfaces.map(item => <div className="interface-row" key={item.name}><div><h3><StatusDot ok={item.up}/>{item.name}</h3><p>{item.addresses.join(' · ') || '未分配 IP 地址'}</p></div><span>MTU {item.mtu}<Icon name="chevron"/></span></div>) || <div className="loading-row">正在读取接口…</div>}</div></SettingGroup>
    <SettingGroup title="网络配置" description="所有更改都会先展示影响，再请求系统授权；远程服务器请保留备用管理连接。"><div className="action-grid"><ActionForm title="DNS 服务器" description="为指定网络接口设置名称解析服务器。" operation="dns-set" actionLabel="配置 DNS" busy={busy} onPlan={onPlan}><Field label="接口" name="interface" hint="例如 en0、eth0"/><Field label="DNS 服务器" name="dns" hint="以空格分隔，例如 1.1.1.1 8.8.8.8"/></ActionForm><ActionForm title="静态 IP 地址" description="为接口添加一个 CIDR 格式的地址。" operation="ip-add" actionLabel="添加地址" busy={busy} onPlan={onPlan}><Field label="接口" name="interface"/><Field label="IP/CIDR" name="cidr" hint="例如 192.168.1.20/24"/></ActionForm><ActionForm title="静态路由" description="将目标网段经指定网关转发。" operation="route-add" actionLabel="添加路由" busy={busy} onPlan={onPlan}><Field label="目标 CIDR" name="cidr"/><Field label="网关" name="gateway"/><Field label="接口（可选）" name="interface"/></ActionForm></div></SettingGroup></div>
}

function Ports({ snapshot, busy, onPlan }: { snapshot?: Snapshot; busy: boolean; onPlan: (operation: string, form: HTMLFormElement) => Promise<void> }) {
  return <div className="settings-stack"><div className="summary-card"><div className="summary-card__icon"><Icon name="ports"/></div><div><p>网络服务</p><h2>服务与流量</h2><span>查看本机流量，并按需配置跨设备的网络转发。</span></div></div>
    <SettingGroup title="流量概览" description="当前操作系统接口计数，仅供查看。"><pre className="traffic-readout">{snapshot?.traffic || '正在读取流量数据…'}</pre></SettingGroup>
    <SettingGroup title="端口转发" description="仅在需要跨设备提供网络服务时配置；端口由你按实际服务填写。"><div className="action-grid action-grid--single"><ActionForm title="新建端口转发" description="将本机某个监听端口转发到指定的网络地址。" operation="forward-add" actionLabel="检查并继续" busy={busy} onPlan={onPlan}><Field label="本机监听端口" name="listenPort" type="number" hint="例如 8080"/><Field label="目标 IP" name="targetIP" hint="例如 192.168.1.100"/><Field label="目标端口" name="targetPort" type="number" hint="留空时使用监听端口"/><Field label="协议" name="protocol" defaultValue="tcp" hint="Windows 当前仅支持 TCP"/></ActionForm></div></SettingGroup></div>
}

function Firewall({ firewall, toggle, status, busy, onPlan }: { firewall?: Capability; toggle?: Capability; status?: FirewallStatus; busy: boolean; onPlan: (operation: string, form: HTMLFormElement) => Promise<void> }) {
  const enabled = status?.enabled
  return <div className="settings-stack"><div className="summary-card"><div className="summary-card__icon"><Icon name="firewall"/></div><div><p>系统安全</p><h2><StatusDot ok={enabled === true}/>{enabled === true ? '防火墙已启用' : enabled === false ? '防火墙已停用' : '正在读取防火墙状态'}</h2><span>{status?.backend ? `由 ${status.backend} 提供保护` : status?.detail || '入站访问由系统防火墙控制。'}</span></div></div>
    {!firewall?.available ? <SettingGroup title="防火墙不可用"><div className="unavailable-row">{firewall?.reason ?? '正在识别当前平台的防火墙能力。'}</div></SettingGroup> : <><SettingGroup title="防火墙开关" description="更改此项会影响所有入站网络连接。">{toggle?.available ? <ActionForm title={enabled === true ? '关闭系统防火墙' : '开启系统防火墙'} description={enabled === true ? '关闭会允许所有入站端口访问，风险较高。' : '开启可让系统根据规则管理入站访问。'} operation="firewall-set" actionLabel={enabled === true ? '关闭防火墙' : '开启防火墙'} busy={busy} disabled={enabled === undefined} onPlan={onPlan}><input type="hidden" name="state" value={enabled ? 'off' : 'on'}/></ActionForm> : <div className="unavailable-row">{toggle?.reason ?? '当前平台不支持管理总开关。'}</div>}</SettingGroup>
      <SettingGroup title="入站端口规则" description="只管理由本插件创建的规则；端口由你按实际服务填写。"><div className="action-grid"><ActionForm title="允许端口访问" description="允许指定协议的入站连接。" operation="open-port" actionLabel="允许端口" busy={busy} onPlan={onPlan}><Field label="端口" name="port" type="number" hint="例如 8080"/><Field label="协议" name="protocol" defaultValue="tcp"/></ActionForm><ActionForm title="阻止端口访问" description="移除本插件创建的允许规则。" operation="close-port" actionLabel="阻止端口" busy={busy} onPlan={onPlan}><Field label="端口" name="port" type="number" hint="例如 8080"/><Field label="协议" name="protocol" defaultValue="tcp"/></ActionForm></div></SettingGroup></>}</div>
}

function PlanModal({ plan, busy, onCancel, onApply }: { plan: Plan; busy: boolean; onCancel: () => void; onApply: () => void }) { return <div className="modal-backdrop"><section className="modal-card"><p className="eyebrow">变更确认</p><h2>{labels[plan.operation] ?? plan.operation}</h2><p className="modal-card__lead">此操作会修改系统设置。请确认影响范围后继续。</p><dl><div><dt>风险等级</dt><dd className={plan.risk === 'high' ? 'risk-high' : 'risk-medium'}>{plan.risk === 'high' ? '高' : '中'}</dd></div><div><dt>影响</dt><dd>{plan.impact}</dd></div><div><dt>参数</dt><dd>{Object.entries(plan.params).map(([key, value]) => `${key}=${value}`).join('，')}</dd></div><div><dt>验证</dt><dd>{plan.verification.join('、')}</dd></div></dl><p className="modal-card__expiry">计划将在 {new Date(plan.expiresAt).toLocaleTimeString()} 失效。</p><div className="modal-actions"><button className="secondary-button" disabled={busy} onClick={onCancel}>取消</button><button className="danger-button" disabled={busy} onClick={onApply}>继续并请求授权</button></div></section></div> }

function PrivilegeRequestModal({ request, busy, onCancel, onExecute }: { request: PrivilegeRequest; busy: boolean; onCancel: () => void; onExecute: (password?: string) => void }) {
  const [password, setPassword] = useState(''); const [confirmation, setConfirmation] = useState(''); const [error, setError] = useState(''); const requiresPassword = request.preflight.authorization === 'password'
  const submit = () => { if (requiresPassword && !password) { setError('请输入当前系统账户的管理员密码。'); return }; if (requiresPassword && password !== confirmation) { setError('两次输入的密码不一致。'); return }; const value = password; setPassword(''); setConfirmation(''); setError(''); onExecute(requiresPassword ? value : undefined) }
  const nativeLabel = request.preflight.authorization === 'native-uac' ? '继续并调起 Windows UAC' : request.preflight.authorization === 'polkit' ? '继续并调起系统授权' : request.preflight.authorization === 'native' ? '继续并调起 macOS 授权' : '确认授权'
  return <div className="modal-backdrop"><section className="modal-card modal-card--small"><p className="eyebrow">系统授权</p><h2>{request.preflight.title}</h2><p className="modal-card__lead">{request.preflight.description}</p>{!request.preflight.available ? <p className="notice notice--warning">{request.preflight.reason || '当前无法请求系统权限。'}</p> : <>{requiresPassword && <div className="form-grid"><label className="form-field"><span>管理员密码</span><input type="password" autoComplete="current-password" value={password} onChange={event => { setPassword(event.target.value); setError('') }}/></label><label className="form-field"><span>确认管理员密码</span><input type="password" autoComplete="current-password" value={confirmation} onChange={event => { setConfirmation(event.target.value); setError('') }}/></label></div>}{(error || request.error) && <p className="notice notice--error">{error || request.error}</p>}</>}<div className="modal-actions"><button className="secondary-button" disabled={busy} onClick={onCancel}>取消</button>{request.preflight.available && <button className="danger-button" disabled={busy} onClick={submit}>{requiresPassword ? '确认授权' : nativeLabel}</button>}</div></section></div>
}
