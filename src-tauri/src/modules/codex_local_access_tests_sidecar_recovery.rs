#[test]
fn sidecar_auth_recovery_preserves_authority_and_token_generation() {
    let mut account = CodexAccount::new(
        "recovery-oauth".to_string(),
        "recovery@example.com".to_string(),
        CodexTokens {
            id_token: "id-test".to_string(),
            access_token: make_test_jwt(json!({"exp": now_ms() / 1000 + 3600})),
            refresh_token: Some("host-only-refresh-secret".to_string()),
        },
    );
    account.token_generation = 42;
    let collection = test_local_access_collection(vec![account.id.clone()]);
    let projected = super::sidecar_auth_json_for_account(&account, &collection, None);
    let reply = super::sidecar_auth_recovery_credentials(&account);
    assert_eq!(projected["refresh_owner"], "cockpit_token_authority");
    assert_eq!(projected["refresh_token"], "");
    assert_eq!(projected["cockpit_token_generation"], 42);
    for key in [
        "access_token",
        "id_token",
        "expired",
        "last_refresh",
        "cockpit_token_generation",
    ] {
        assert_eq!(
            reply[key], projected[key],
            "recovery and startup differ for {key}"
        );
    }
    assert!(reply.get("refresh_token").is_none());
    assert!(!reply.to_string().contains("host-only-refresh-secret"));
}

#[test]
fn sidecar_auth_recovery_preserves_quota_and_reauth_restrictions() {
    let mut account = CodexAccount::new(
        "recovery-guard".to_string(),
        "recovery@example.com".to_string(),
        CodexTokens {
            id_token: String::new(),
            access_token: make_test_jwt(json!({"exp": now_ms() / 1000 + 3600})),
            refresh_token: Some("refresh-test".to_string()),
        },
    );
    assert!(super::sidecar_auth_recovery_allowed(&account));
    account.requires_reauth = true;
    assert!(!super::sidecar_auth_recovery_allowed(&account));
    account.requires_reauth = false;
    account.tokens.refresh_token = None;
    assert!(!super::sidecar_auth_recovery_allowed(&account));
    account.tokens.refresh_token = Some("refresh-test".to_string());
    account.quota = Some(
        serde_json::from_value(json!({
            "hourly_percentage": 0,
            "weekly_percentage": 50,
            "hourly_reset_time": now_ms() / 1000 + 3600,
            "hourly_window_present": true,
        }))
        .unwrap(),
    );
    assert!(!super::sidecar_auth_recovery_allowed(&account));
    account.quota.as_mut().unwrap().hourly_reset_time = None;
    assert!(!super::sidecar_auth_recovery_allowed(&account));
}

#[cfg(unix)]
#[tokio::test]
async fn sidecar_auth_recovery_rejects_unscoped_account_over_private_pipe() {
    use super::{timeout, Arc, AsyncBufReadExt, BufReader, Stdio, TokioCommand, TokioMutex};
    let mut child = TokioCommand::new("/bin/cat")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .unwrap();
    let stdin = Arc::new(TokioMutex::new(child.stdin.take().unwrap()));
    let mut output = BufReader::new(child.stdout.take().unwrap()).lines();
    let request = super::SidecarAuthRecoveryRequest {
        recovery_id: "test-request".to_string(),
        account_id: "outside-the-launch-scope".to_string(),
        reason: "credentials".to_string(),
        observed_generation: 0,
    };
    super::handle_sidecar_auth_recovery(request, Arc::new(HashSet::new()), stdin).await;
    let line = timeout(Duration::from_secs(2), output.next_line())
        .await
        .unwrap()
        .unwrap()
        .unwrap();
    let reply: Value = serde_json::from_str(&line).unwrap();
    assert_eq!(reply["type"], "auth_recovery_result");
    assert_eq!(reply["recoveryId"], "test-request");
    assert_eq!(reply["success"], false);
    assert_eq!(reply["errorCode"], "invalid_recovery_request");
    assert!(reply.get("credentials").is_none());
    child.kill().await.unwrap();
    let _ = child.wait().await;
}
