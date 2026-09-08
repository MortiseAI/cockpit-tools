#[test]
fn fixed_local_access_endpoint_migrates_existing_config_and_preserves_named_keys() {
    let mut collection = super::new_empty_local_access_collection().unwrap();
    collection.port = 1456;
    collection.client_base_url_host = super::CodexLocalAccessClientBaseUrlHost::Ipv4Loopback;
    let mut primary = super::build_local_access_api_key(Some("Existing primary"));
    primary.token_used = 42;
    primary.account_ids = vec!["account-1".to_string()];
    primary.inherit_account_pool = Some(false);
    let secondary = super::build_local_access_api_key(Some("Secondary"));
    collection.api_key = primary.key.clone();
    collection.api_keys = vec![primary.clone(), secondary.clone()];

    assert!(super::enforce_fixed_local_access_endpoint(&mut collection));
    assert_eq!(
        super::build_collection_base_url(&collection),
        "http://localhost:52340/v1"
    );
    assert_eq!(collection.api_key, super::CODEX_LOCAL_ACCESS_FIXED_API_KEY);
    assert_eq!(collection.api_keys[0].id, primary.id);
    assert_eq!(collection.api_keys[0].label, primary.label);
    assert_eq!(collection.api_keys[0].token_used, 42);
    assert_eq!(collection.api_keys[0].account_ids, primary.account_ids);
    assert_eq!(collection.api_keys[1].id, secondary.id);
    assert_eq!(collection.api_keys[1].key, secondary.key);
    assert!(super::resolve_collection_api_key(
        &collection,
        super::CODEX_LOCAL_ACCESS_FIXED_API_KEY
    )
    .is_some());
    assert!(super::resolve_collection_api_key(&collection, &primary.key).is_none());
    assert!(!super::enforce_fixed_local_access_endpoint(&mut collection));
}

#[test]
fn fixed_local_access_endpoint_restores_disabled_primary_without_duplicate_keys() {
    let mut collection = super::new_empty_local_access_collection().unwrap();
    let secondary = super::build_local_access_api_key(Some("Secondary"));
    let mut primary = super::build_local_access_api_key(Some("Fixed primary"));
    primary.key = super::CODEX_LOCAL_ACCESS_FIXED_API_KEY.to_string();
    primary.enabled = false;
    collection.api_key = secondary.key.clone();
    collection.api_keys = vec![secondary.clone(), primary.clone()];

    assert!(super::enforce_fixed_local_access_endpoint(&mut collection));
    assert_eq!(collection.api_keys.len(), 2);
    assert_eq!(collection.api_keys[0].id, primary.id);
    assert!(collection.api_keys[0].enabled);
    assert_eq!(collection.api_keys[1].key, secondary.key);
    assert_eq!(collection.api_key, super::CODEX_LOCAL_ACCESS_FIXED_API_KEY);
    assert!(!super::enforce_fixed_local_access_endpoint(&mut collection));
}

#[test]
fn fixed_local_access_endpoint_applies_to_new_and_runtime_collections() {
    let collection = super::new_local_access_collection().unwrap();
    assert_eq!(
        super::build_collection_base_url(&collection),
        "http://localhost:52340/v1"
    );
    assert_eq!(collection.api_key, super::CODEX_LOCAL_ACCESS_FIXED_API_KEY);

    let mut runtime = super::GatewayRuntime::default();
    let mut stale_collection = super::new_empty_local_access_collection().unwrap();
    stale_collection.port = 12345;
    super::sync_runtime_collection(&mut runtime, stale_collection);
    let restored = runtime.collection.unwrap();
    assert_eq!(
        super::build_collection_base_url(&restored),
        "http://localhost:52340/v1"
    );
    assert_eq!(restored.api_key, super::CODEX_LOCAL_ACCESS_FIXED_API_KEY);
    assert_eq!(restored.api_keys[0].key, restored.api_key);
}

#[tokio::test]
async fn fixed_local_access_endpoint_rejects_port_host_and_primary_key_changes() {
    assert!(super::update_local_access_port(1456).await.is_err());
    assert!(super::update_local_access_port(0).await.is_err());
    assert!(super::update_local_access_client_base_url_host(
        super::CodexLocalAccessClientBaseUrlHost::Ipv4Loopback,
    )
    .await
    .is_err());
    assert!(super::rotate_local_access_api_key().await.is_err());
}
