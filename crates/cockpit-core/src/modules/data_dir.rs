//! Shared Cockpit storage location, independent of the application's dev/prod branding.

use std::path::PathBuf;

pub const DEFAULT_DATA_DIR: &str = ".antigravity_cockpit";

fn resolve_path(override_dir: Option<&str>, home: Option<PathBuf>) -> Result<PathBuf, String> {
    if let Some(path) = override_dir.map(str::trim).filter(|path| !path.is_empty()) {
        return Ok(PathBuf::from(path));
    }
    home.map(|home| home.join(DEFAULT_DATA_DIR))
        .ok_or_else(|| "无法获取用户主目录".to_string())
}

pub fn resolve_data_dir() -> Result<PathBuf, String> {
    // Profiles must not select storage. Isolation must be requested explicitly
    // with DATA_DIR instead of creating another store for development builds.
    let override_dir = std::env::var("COCKPIT_TOOLS_DATA_DIR").ok();
    resolve_path(override_dir.as_deref(), dirs::home_dir())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn defaults_to_shared_directory() {
        let home = PathBuf::from("fixture-home");
        for override_dir in [None, Some(""), Some(" \t ")] {
            assert_eq!(
                resolve_path(override_dir, Some(home.clone())).unwrap(),
                home.join(".antigravity_cockpit")
            );
        }
    }

    #[test]
    fn explicit_directory_does_not_require_a_home() {
        assert_eq!(
            resolve_path(Some("  fixture-custom  "), None).unwrap(),
            PathBuf::from("fixture-custom")
        );
        assert!(resolve_path(None, None).is_err());
    }

    #[test]
    fn profiles_use_the_same_directory_in_separate_processes() {
        const CHILD_MARKER: &str = "COCKPIT_DATA_DIR_TEST_CHILD";
        if std::env::var_os(CHILD_MARKER).is_some() {
            let expected = dirs::home_dir().unwrap().join(DEFAULT_DATA_DIR);
            assert_eq!(resolve_data_dir().unwrap(), expected);
            return;
        }
        for profile in ["dev", "prod", " DEV ", ""] {
            let result = std::process::Command::new(std::env::current_exe().unwrap())
                .args(["--exact", "modules::data_dir::tests::profiles_use_the_same_directory_in_separate_processes"])
                .env(CHILD_MARKER, "1")
                .env("COCKPIT_TOOLS_PROFILE", profile)
                .env_remove("COCKPIT_TOOLS_DATA_DIR")
                .output()
                .unwrap();
            assert!(
                result.status.success(),
                "profile {profile:?}: {}",
                String::from_utf8_lossy(&result.stderr)
            );
            assert!(String::from_utf8_lossy(&result.stdout).contains("1 passed"));
        }
    }
}
