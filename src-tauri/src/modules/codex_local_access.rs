// Codex Local Access 统一入口。
// 业务分片只在完整顶层 item 之间切开，通过 include! 保持同一模块作用域；
// 生产流量由 Sidecar 处理。旧 Rust 网关的协议与路由辅助代码以 cfg(test)
// 保留用于回归测试；主进程仍使用的账号管理、配置、统计及刷新逻辑正常编译。
include!("codex_local_access_foundation.rs");
include!("codex_local_access_quota_cooldown.rs");
include!("codex_local_access_request_transform.rs");
include!("codex_local_access_routing_pricing.rs");
include!("codex_local_access_request_logs.rs");
include!("codex_local_access_profile_takeover.rs");
include!("codex_local_access_sidecar_config.rs");
include!("codex_local_access_sidecar_recovery.rs");
include!("codex_local_access_sidecar_runtime.rs");
include!("codex_local_access_collection.rs");
include!("codex_local_access_gateway_runtime.rs");
include!("codex_local_access_provider_gateway.rs");
include!("codex_local_access_probe_chat.rs");
include!("codex_pelican_transport.rs");
include!("codex_local_access_commands.rs");
include!("codex_local_access_http.rs");
// The retired in-process WebSocket gateway is retained only as a test oracle.
// Production traffic is handled by the bundled CLIProxyAPI sidecar.
#[cfg(test)]
include!("codex_local_access_recovery.rs");

#[cfg(test)]
mod tests {
    include!("codex_local_access_tests_fixed_endpoint.rs");
    include!("codex_local_access_tests_sidecar_gateway.rs");
    include!("codex_local_access_tests_sidecar_recovery.rs");
    include!("codex_local_access_tests_pricing_profile.rs");
    include!("codex_local_access_tests_request_routing.rs");
    include!("codex_local_access_tests_takeover.rs");
}
