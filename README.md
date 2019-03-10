# Wattchecker Exporter

ワットチェッカー(REX-BTWATTCH1)用Prometheusのexporterです．電圧，電流，電力量を取得します．

## 事前準備(Bluetooth接続編)

1. 必要ソフトのインストール
    ```
    $ sudo apt-get install -y bluetooth
    $ sudo apt-get install -y pi-bluetooth
    ```
1. Bluetoothバージョン確認
    ```
    $ bluetoothctl -v
    ```
1. Bluetoothの設定

    ```
    $ sudo bluetoothctl
    ```
1. 内蔵Bluetoothの電源を入れる
    ```
    [bluetooth]# power on
    ```
    1. エージェントを起動する
        ```
        [bluetooth]# agent on
        ```
    1. スキャン  
        WATT CHECKERと表示されるもののアドレス(MACアドレス？)をコピー
        ```
        [bluetooth]# scan on
        ```
    1. ペアリングを行う  
        パスワードは`0000`
        ```
        [bluetooth]# pair アドレス
        ```
    1. trastを行う
        ```
        [bluetooth]# trust アドレス
        ```
    1. 詳細情報表示
        ```
        [bluetooth]# info アドレス
        ```
    1. 一度終了
        ```
        [bluetooth]# exit
        ```
1. sdptoolの設定(SDPサーバの準備)  
    1. bluetooth.serviceを書き換え
        ```
        sudo vi /lib/systemd/system/bluetooth.service
        ```
        下の内容を追記する(`bluetooth.service`)
        ```
        #ExecStart=/usr/lib/bluetooth/bluetoothd
        ExecStart=/usr/lib/bluetooth/bluetoothd -C
        ExecStartPost=/usr/bin/sdptool add SP
        ```
    1. サービスの再起動
        ```
        $ sudo systemctl daemon-reload
        $ sudo systemctl restart bluetooth
        ```
1. rc.localの書き換え
    ```
    $ sudo vi /etc/rc.local
    ```
    下の内容を追記する(`rc.local`)
    ```
    # Setup rfcomm
    sudo rfcomm bind /dev/rfcomm0 00:0C:BF:20:12:8A 6
    sudo chmod 777 /var/run/sdp
    sdptool add --channel=22 SP)
    ```
1. 端末の再起動を行う
    ```
    $ sudo reboot
    ```
1. 再起動後，シリアルポートが出来ているか確認  
(自分の環境ではいっぱい出てきました)
    ```
    $ sdptool browse local|grep -i serial
    Service Name: Serial Port
    "Serial Port" (0x1101)
    "Serial Port" (0x1101)
    Service Name: Serial Port
    "Serial Port" (0x1101)
    "Serial Port" (0x1101)
    ```
1. 接続状態の確認
    ```
    $ rfcomm show 0
    rfcomm0: 00:0C:BF:20:12:8A channel 6 connected [tty-attached]
    ```

[参考サイト(IoT+)](https://iot-plus.net/make/raspi/24h-watt-monitoring-using-rex-btwattch1/)  
[参考サイト(アットマークテクノ)](https://armadillo.atmark-techno.com/howto/armadillo_rex-btwattch1)

## 事前準備(GO実行環境構築編)
1. パッケージのダウンロード  
※環境に合わせてください
    ```
    $ wget https://dl.google.com/go/go1.12.linux-armv6l.tar.gz
    ```
1. パッケージの展開  
    少し時間がかかるので気長に待ちます
    ```
    $ sudo tar -C /usr/local -xvzf go1.12.linux-armv6l.tar.gz
    ```
1. インストール確認
    ```
    $ ll /usr/local/go
    合計 216K
    drwxr-xr-x 10 root root  4.0K  2月 26 08:05 .
    drwxrwsr-x 13 root staff 4.0K 12月 22 00:10 ..
    -rw-r--r--  1 root root   55K  2月 26 08:05 AUTHORS
    -rw-r--r--  1 root root  1.4K  2月 26 08:05 CONTRIBUTING.md
    -rw-r--r--  1 root root   77K  2月 26 08:05 CONTRIBUTORS
    -rw-r--r--  1 root root  1.5K  2月 26 08:05 LICENSE
    -rw-r--r--  1 root root  1.3K  2月 26 08:05 PATENTS
    -rw-r--r--  1 root root  1.6K  2月 26 08:05 README.md
    -rw-r--r--  1 root root     6  2月 26 08:05 VERSION
    drwxr-xr-x  2 root root  4.0K  2月 26 08:05 api
    drwxr-xr-x  2 root root  4.0K  2月 26 08:20 bin
    drwxr-xr-x  8 root root  4.0K  2月 26 08:05 doc
    -rw-r--r--  1 root root  5.6K  2月 26 08:05 favicon.ico
    drwxr-xr-x  3 root root  4.0K  2月 26 08:05 lib
    drwxr-xr-x 15 root root  4.0K  2月 26 08:05 misc
    drwxr-xr-x  5 root root  4.0K  2月 26 08:20 pkg
    -rw-r--r--  1 root root    26  2月 26 08:05 robots.txt
    drwxr-xr-x 47 root root  4.0K  2月 26 08:05 src
    drwxr-xr-x 22 root root   12K  2月 26 08:05 test
    ```
1. バージョン確認
    ```
    $ cat /usr/local/go/VERSION
    go1.12
    ```
1. パス設定
    ```
    $ vi .bashrc
    ```
    下の内容を最後に追記する．
    ```
    # GOPATHの設定
    export GOPATH=$HOME/go
    # Go言語のPATHを通す
    export PATH=$PATH:/usr/local/go/bin
    ```
1. パス設定変更の反映
    ```
    $ source .bashrc
    ```
1. goコマンドからバージョン確認
    ```
    $ go version
    go version go1.12 linux/arm
    ```
1. GOPATHの確認
    ```
    $ go env GOPATH
    /home/XXXXXX/go
    ```

## 事前準備(ビルド～仮実行)

1. GOPATHへ移動  
    ディレクトリがない場合は適宜作成
    ```
    $ cd go
    ```
1. リポジトリ用のディレクトリ作成
    ```
    $ mkdir repository
    ```
1. リポジトリ用のディレクトリに移動
    ```
    $ cd repository/
    ```
1. リポジトリのクローン
    ```
    $ git clone https://github.com/Kisogawa/wattchecker_exporter.git
    ```
1. リポジトリに移動
    ```
    $ cd wattchecker_exporter/
    ```
1. デバイスのパスが違う場合はここでソースコードを修正  
    (デバイスのパスは63行目あたりの変数DevicePath)

1. ビルドを行う
    ```
    $ go build wattchecker_exporter.go 
    ```
1. 試しに実行
    ```
    $ ./wattchecker_exporter
    シリアルポート接続[/dev/rfcomm0]...完了
    ワットチェッカーの初期化開始...完了
    計測開始中...完了
    サーバーを公開します
    ```
1. ブラウザから確認  
    何か表示されればOK
    ```
    http://ホスト名:4351/metrics
    ```

## 自動実行の設定

1. ビルドした実行ファイルをbinにコピー
    ```
    $ sudo cp go/repository/wattchecker_exporter/wattchecker_exporter /usr/local/bin/
    ```

1. サービス起動用ファイル作成
    ```
    $ sudo vi /etc/systemd/system/wattchecker_exporter.service
    ```
    下の内容をファイルに書き込む
    ```
    [Unit]
    Description = wattchecker_exporter
    After=multi-user.target
    
    [Service]
    ExecStart = /usr/local/bin/wattchecker_exporter
    Restart = always
    Type = simple
    
    [Install]
    WantedBy = multi-user.target
    ```
1. サービスを有効にする
    ```
    $ sudo systemctl daemon-reload
    $ sudo systemctl enable /etc/systemd/system/wattchecker_exporter.service
    ```
1. サービスの状態を確認する
    ```
    $ systemctl status wattchecker_exporter.service
    ```
1. サービスを起動する
    ```
    $ sudo systemctl start wattchecker_exporter.servic
    ```
1. サービスの起動状態が`active (running)`になっていればOK!!  
    以上でPrometheusから情報収集出来るようになります