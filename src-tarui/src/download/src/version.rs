use std::path::Path;

use crate::{get, Download};
use model::version_manifest::Version;

// 为Version结构体实现Download特性，用于下载Minecraft游戏版本
impl Download for Version {
    
    // 下载方法：接收游戏目录路径，下载并保存指定版本的游戏文件
    fn download(&self, game_dir: &Path) -> Result<(), Box<dyn std::error::Error>> {
        println!("开始下载游戏内容,路径:{}", game_dir.to_str().unwrap());
        // 打印 Version
        println!("Version:{}", self.id);
        println!("Version Type:{}", self.type_);
        println!("Version URL:{}", self.url);


        // 从版本URL获取详细版本信息并解析为Version结构体
        let game = get(&self.url)?.json::<model::version::Version>()?;
        // 打印获取的游戏版本信息
        println!("Game ID:{}", game.id);
        println!("Game Type:{}", game.type_);
        println!("Game Time:{}", game.time);
        println!("Game Release Time:{}", game.release_time);
        println!("Game Asset Index:{}", game.asset_index.id);
        println!("Game Asset Index URL:{}", game.asset_index.url);
        println!("Game Asset Index SHA1:{}", game.asset_index.sha1);
        println!("Game Asset Index Size:{}", game.asset_index.size);
        println!("Game Libraries:{}", game.libraries.len());

        // 构建版本目录路径：game_dir/versions/游戏ID
        let versions_dir = &game_dir.join("versions").join(&game.id);

        // 如果版本目录不存在，则创建它
        if !versions_dir.exists() {
            std::fs::create_dir_all(versions_dir)?;
        }
        // 下载该版本需要的所有库文件
        println!("开始下载libraries");
        game.libraries.download(game_dir)?;
        // 下载游戏资源索引文件
        println!("开始下载assets");
        game.asset_index.download(game_dir)?;

        // 构建版本配置文件路径：game_dir/versions/游戏ID/游戏ID.json
        let version_config = &game_dir
            .join("versions")
            .join(&game.id)
            .join(&format!("{}.json", &self.id));

        // 如果版本配置文件已存在
        if version_config.exists() {
             std::fs::remove_file(version_config)?;
             println!("删除旧版本配置文件:{}", version_config.display());
        }

        // 创建版本配置文件
        std::fs::File::create(version_config)?;
        // 将从URL获取的版本信息写入配置文件
        std::fs::write(version_config, get(&self.url)?.bytes()?)?;

        // 构建游戏JAR文件路径：game_dir/versions/游戏ID/游戏ID.jar
        let path = &game_dir
            .join("versions")
            .join(&game.id)
            .join(&format!("{}.jar", &game.id));

        // 如果JAR文件已存在，验证其SHA1哈希值
        if path.exists() {
            println!("JAR文件已存在:{}", path.display());
            // 如果哈希值匹配，表示文件已正确下载，直接返回成功
            if crate::sha1(path)?.eq(&game.downloads.client.sha1) {
                println!("JAR文件SHA1匹配,无需重新下载");
                return Ok(());
            } else {
                // 哈希值不匹配，删除现有文件以便重新下载
                println!("JAR文件SHA1不匹配,删除旧文件");
                std::fs::remove_file(path)?;
            }
        }

        // 创建新的JAR文件
        std::fs::File::create(path)?;
        
        // 从客户端下载URL获取游戏JAR文件内容
        let bytes = get(&game.downloads.client.url)?.bytes()?;

        // 将下载的内容写入JAR文件
        std::fs::write(path, bytes)?;

        // 所有操作成功完成
        Ok(())
    }
}

// 测试模块，仅在运行测试时编译
#[cfg(test)]
mod tests {
    use super::*;

    // 测试下载功能是否正常工作
    #[test]
    fn test_download() {
        // 创建测试版本对象，使用Minecraft 1.21版本信息
        let version = Version {
            id: "1.21".to_string(),
            type_: "release".to_string(),
            url: "https://piston-meta.mojang.com/v1/packages/177e49d3233cb6eac42f0495c0a48e719870c2ae/1.21.json".to_string(),
            time : "2024-06-13T08:32:38+00:00".to_string(),
            release_time : "2024-06-13T08:24:03+00:00".to_string(),
        };

        // 在临时目录中创建测试下载目录
        let download_path = &std::env::temp_dir().join("rust-minecraft-client-launch");
        std::fs::create_dir_all(download_path).unwrap_or_else(|err| panic!("{:?}", err));

        // 执行下载测试，如果失败则抛出错误
        if let Err(err) = version.download(download_path) {
            panic!("{:?}", err);
        }
    }
}
