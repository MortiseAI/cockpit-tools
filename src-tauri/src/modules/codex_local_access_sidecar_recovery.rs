// The sidecar never owns refresh_token. Recovery uses the existing Token
// Authority and acknowledges the bearer snapshot over the private stdin pipe.
#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SidecarAuthRecoveryRequest {
    recovery_id: String,
    account_id: String,
    reason: String,
    observed_generation: u64,
}

#[derive(Default)]
struct SidecarAuthRecoveryBackoff {
    finished_at: Option<Instant>,
    failures: u32,
}

impl SidecarAuthRecoveryBackoff {
    fn cooling_down(&self) -> bool {
        let seconds = 60_u64
            .saturating_mul(1 << self.failures.saturating_sub(1).min(3))
            .min(300);
        self.finished_at
            .is_some_and(|finished| finished.elapsed() < Duration::from_secs(seconds))
    }

    fn finish(&mut self, success: bool) {
        self.finished_at = Some(Instant::now());
        self.failures = if success {
            0
        } else {
            self.failures.saturating_add(1).min(4)
        };
    }
}

type SidecarAuthRecoveryLocks = Mutex<HashMap<String, Arc<TokioMutex<SidecarAuthRecoveryBackoff>>>>;

fn sidecar_auth_recovery_lock(account_id: &str) -> Arc<TokioMutex<SidecarAuthRecoveryBackoff>> {
    static LOCKS: OnceLock<SidecarAuthRecoveryLocks> = OnceLock::new();
    let mut locks = LOCKS
        .get_or_init(|| Mutex::new(HashMap::new()))
        .lock()
        .unwrap_or_else(|e| e.into_inner());
    Arc::clone(locks.entry(account_id.to_string()).or_default())
}

fn sidecar_auth_recovery_allowed(account: &CodexAccount) -> bool {
    !account.requires_reauth
        && !account.is_api_key_auth()
        && !account.is_agent_identity_auth()
        && !account_is_access_token_only(account)
        && !account_quota_cooldown(account, now_ms()).is_some_and(|quota| quota.active(now_ms()))
}

fn sidecar_auth_recovery_credentials(account: &CodexAccount) -> Value {
    json!({
        "access_token": account.tokens.access_token,
        "id_token": account.tokens.id_token,
        "expired": codex_oauth::jwt_token_expiration_timestamp(&account.tokens.access_token),
        "last_refresh": sidecar_account_last_refresh(account),
        "cockpit_token_generation": account.token_generation,
    })
}

async fn recover_sidecar_account(request: SidecarAuthRecoveryRequest) -> Result<Value, String> {
    let lock = sidecar_auth_recovery_lock(&request.account_id);
    let mut backoff = lock.lock().await;
    let current = codex_account::load_account(&request.account_id).ok_or("account_missing")?;
    if !sidecar_auth_recovery_allowed(&current) {
        return Err("account_not_recoverable".to_string());
    }
    if backoff.cooling_down() {
        // A second sidecar can consume the first one's completed refresh, but
        // cannot rotate the same generation again inside the backoff window.
        if backoff.failures == 0
            && (current.token_generation > request.observed_generation
                || request.reason == "transient")
            && !codex_oauth::is_token_expired(&current.tokens.access_token)
        {
            return Ok(sidecar_auth_recovery_credentials(&current));
        }
        return Err("recovery_backoff".to_string());
    }
    let refreshed = if request.reason == "credentials" {
        codex_account::force_refresh_managed_account_after_observed(
            &request.account_id,
            request.observed_generation,
            "sidecar_auth_unavailable",
        )
        .await
    } else {
        codex_account::ensure_managed_account_fresh(&request.account_id).await
    };
    let result = match refreshed {
        Ok(account)
            if sidecar_auth_recovery_allowed(&account)
                && !codex_oauth::is_token_expired(&account.tokens.access_token) =>
        {
            // Also update disk so a sidecar restart consumes the same snapshot.
            // The reply below is the synchronous acknowledgement for this request.
            sync_sidecar_auth_file_for_account(&account)
                .map(|()| sidecar_auth_recovery_credentials(&account))
                .map_err(|_| "auth_sync_failed".to_string())
        }
        Ok(_) => Err("account_not_recoverable".to_string()),
        // Token Authority records the detailed error. Do not echo credentials
        // or provider response bodies onto either sidecar pipe.
        Err(_) => Err("credential_refresh_failed".to_string()),
    };
    backoff.finish(result.is_ok());
    result
}

async fn handle_sidecar_auth_recovery(
    request: SidecarAuthRecoveryRequest,
    allowed_accounts: Arc<HashSet<String>>,
    stdin: Arc<TokioMutex<tokio::process::ChildStdin>>,
) {
    let result = if !allowed_accounts.contains(&request.account_id)
        || request.recovery_id.is_empty()
        || request.recovery_id.len() > 128
        || !matches!(request.reason.as_str(), "credentials" | "transient")
    {
        Err("invalid_recovery_request".to_string())
    } else {
        // A slow OAuth refresh must finish saving its rotated refresh_token
        // even if the API caller times out. Dropping a JoinHandle detaches it.
        let refresh = tokio::spawn(recover_sidecar_account(request.clone()));
        match timeout(Duration::from_secs(8), refresh).await {
            Ok(Ok(result)) => result,
            Ok(Err(_)) => Err("recovery_task_failed".to_string()),
            Err(_) => Err("recovery_timeout".to_string()),
        }
    };
    let mut reply = json!({
        "type": "auth_recovery_result",
        "recoveryId": request.recovery_id,
        "accountId": request.account_id,
        "success": result.is_ok(),
    });
    match result {
        Ok(credentials) => reply["credentials"] = credentials,
        Err(code) => reply["errorCode"] = json!(code),
    }
    if let Ok(mut bytes) = serde_json::to_vec(&reply) {
        bytes.push(b'\n');
        let _ = timeout(Duration::from_secs(1), async {
            let mut writer = stdin.lock().await;
            writer.write_all(&bytes).await?;
            writer.flush().await
        })
        .await;
    }
}
