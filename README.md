# Wattchecker Exporter

ワットチェッカー(REX-BTWATTCH1)用Prometheusのexporterです．電圧，電流，電力量を取得します．

事前準備:

必要ソフトのインストール
```
$ sudo apt-get install -y bluetooth
$ sudo apt-get install -y pi-bluetooth
```
Bluetoothバージョン確認
```
$ bluetoothctl -v
```
Bluetoothの設定
```
$ sudo bluetoothctl
内蔵Bluetoothの電源を入れる
[bluetooth]# power on
エージェントを起動する
[bluetooth]# agent on
スキャン
[bluetooth]# scan on
ペアリングを行う
[bluetooth]# pair <対象機器のアドレス>
trastを行う
[bluetooth]# trust <対象機器のアドレス>
詳細情報表示
[bluetooth]# info <対象機器のアドレス>
一度終了
[bluetooth]# exit
```
sdptoolの設定(SDPサーバの準備)  
bluetooth.serviceを書き換え
```
sudo vi /lib/systemd/system/bluetooth.service
```
bluetooth.service(一部)
```
#ExecStart=/usr/lib/bluetooth/bluetoothd
ExecStart=/usr/lib/bluetooth/bluetoothd -C
ExecStartPost=/usr/bin/sdptool add SP
```
サービスの再起動
```
$ sudo systemctl daemon-reload
$ sudo systemctl restart bluetooth
```
rc.localの書き換え
```
$ sudo vi /etc/rc.local
```
rc.local(追記
```
# Setup rfcomm
sudo rfcomm bind /dev/rfcomm0 00:0C:BF:20:12:8A 6
sudo chmod 777 /var/run/sdp
sdptool add --channel=22 SP)
```
端末の再起動を行う

シリアルポートが出来ているか確認
```
$ sdptool browse local|grep -i serial
```
接続状態
```
$ rfcomm show 0
```

[参考サイト](https://iot-plus.net/make/raspi/24h-watt-monitoring-using-rex-btwattch1/)