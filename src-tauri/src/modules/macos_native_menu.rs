#[cfg(target_os = "macos")]
mod imp {
    include!("macos_native_menu_types.rs");
    include!("macos_native_menu_quota.rs");
    include!("macos_native_menu_platform_cards.rs");
    include!("macos_native_menu_actions.rs");
}

#[cfg(target_os = "macos")]
pub(crate) use imp::{toggle_tray_menu, update_status_item};
