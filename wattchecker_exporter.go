package main

import (
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/goburrow/serial"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sigurn/crc8"

	yaml "gopkg.in/yaml.v2"
)

// 定数の定義
const HDR_CMD uint8 = 0xAA
const RTC_TMR_SND_CMD uint8 = 0x01
const SRT_MSR_SND_CMD uint8 = 0x02
const RTM_MSR_SND_CMD uint8 = 0x08
const HDR_CNT_CRC_LENGTH int = 4
const RTM_MSR_SND_LENGTH int = 1 + HDR_CNT_CRC_LENGTH
const SRT_MSR_SND_LENGTH int = 2 + HDR_CNT_CRC_LENGTH
const RTC_TMR_RCV_LENGTH int = 2 + HDR_CNT_CRC_LENGTH
const SRT_MSR_RCV_LENGTH int = 2 + HDR_CNT_CRC_LENGTH
const RTC_TMR_SND_LENGTH int = 8 + HDR_CNT_CRC_LENGTH
const RTC_MSR_RCV_LENGTH int = 17 + HDR_CNT_CRC_LENGTH
const BUF_SIZE int = 256

const VOLTAGE_STEP float32 = 1           /* mV */
const POWER_STEP float32 = 5             /* mW */
const CURRENT_STEP float32 = (1.0 / 128) /* mA */

// 収集データ記録用構造体
type CollectionData struct {
	lastCollectDate time.Time // 最終取得時間
	time            time.Time // 時間
	voltage         float32   // 電圧(V)
	current         float32   // 電流(mA)
	power           float32   // 電力(W)
}

// 設定ファイル記録用構造体
type Setting struct {
	Devices []Device `yaml:"Devices"`
}
type Device struct {
	DevicePath         string `yaml:"DevicePath"`
	DeviceName         string `yaml:"DeviceName"`
	Port               serial.Port
	wattDurations      prometheus.GaugeFunc
	voltageDurations   prometheus.GaugeFunc
	ampereDurations    prometheus.GaugeFunc
	collectionDataLast CollectionData // 直前の取得データ
}

var (
	addr              = flag.String("listen-address", ":4351", "The address to listen on for HTTP requests.")
	oscillationPeriod = flag.Duration("oscillation-period", 10*time.Minute, "The duration of the rate oscillation period.")
)

var (
	// CRC8定義(Poly以外適当な値)
	CRC8_POLYNOMIAL = crc8.Params{0x85, 0x00, false, false, 0x00, 0xF4, "CRC-8"}
)

// パッケージの初期化時に呼ばれる
func init() {}

func main() {

	// 実行ファイルのパスを取得する
	ownPath, ExecutableErr := os.Executable()
	if ExecutableErr != nil {
		log.Printf("自身のパス取得に失敗[" + ExecutableErr.Error() + "]")
	}

	// ディレクトリ取得
	ownDir := filepath.Dir(ownPath)

	// 設定ファイルを読み込む
	buf, err := ioutil.ReadFile(ownDir + "/setting.yml")
	if err != nil {
		log.Printf("設定ファイル読み込みエラー[" + err.Error() + "]")
		return
	}

	// 構造体に変換する
	var setting Setting
	err = yaml.Unmarshal(buf, &setting)
	if err != nil {
		log.Printf("設定ファイル解釈エラー[" + err.Error() + "]")
		return
	}

	flag.Parse()

	// Deviceの個数分だけ接続する
	for i := 0; i < len(setting.Devices); i++ {
		err := setting.Devices[i].initDevice()
		if err != nil {
			return
		}
		defer setting.Devices[i].finalizeDevice()
	}

	// CRC8テーブル作成
	crc8Table := crc8.MakeTable(CRC8_POLYNOMIAL)

	// ワットチェッカーの初期化を行う
	for i := 0; i < len(setting.Devices); i++ {
		setting.Devices[i].initWattChecker(crc8Table)
	}

	// ワットチェッカーの計測コマンド(計測開始コマンド)
	for i := 0; i < len(setting.Devices); i++ {
		err := setting.Devices[i].startMeasure(crc8Table)
		if err != nil {
			return
		}
	}

	// コレクターの生成
	for i := 0; i < len(setting.Devices); i++ {
		setting.Devices[i].makeCollector(crc8Table)
	}

	// Prometiusに登録する
	for i := 0; i < len(setting.Devices); i++ {
		prometheus.MustRegister(setting.Devices[i].wattDurations)
		prometheus.MustRegister(setting.Devices[i].voltageDurations)
		prometheus.MustRegister(setting.Devices[i].ampereDurations)
	}

	log.Printf("サーバーを公開します")
	// Expose the registered metrics via HTTP.
	// 登録されたmetricをHTTPサーバーで公開する
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*addr, nil))
	log.Printf("サーバー停止")
}

// デバイスの初期化
func (d *Device) initDevice() (err error) {
	devicePath := d.DevicePath
	log.Printf("シリアルポート接続[" + devicePath + "]...")
	// シリアルポートを読み込む(10秒でタイムアウト)
	p, e := serial.Open(&serial.Config{Address: devicePath, Timeout: 10 * time.Second})
	if err != nil {
		log.Fatal(err)
		log.Printf("失敗")
		log.Print(err)
		err = e
		return
	}
	log.Printf("ポート接続完了")
	d.Port = p
	return
}

// ワットチェッカーの初期化を行う
func (d *Device) initWattChecker(crc8Table *crc8.Table) {
	log.Printf("ワットチェッカーの初期化開始[" + d.DevicePath + "]...")
	init_wattchecker(d.Port, crc8Table)
	log.Printf("完了")
}

// ワットチェッカーに計測開始コマンドを出力する
func (d *Device) startMeasure(crc8Table *crc8.Table) (err error) {
	if ret := start_measure(d.Port, crc8Table); ret != 0 {
		log.Printf("計測開始コマンドエラー")
		err = errors.New("計測開始コマンドエラー")
		return
	}
	return
}

// コレクター作成
func (d *Device) makeCollector(crc8Table *crc8.Table) {
	// 電力の取得
	d.wattDurations = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name:        "REXBTWATTCH_Watt",
			Help:        "REX-BTWATTCH",
			ConstLabels: prometheus.Labels{"Name": d.DeviceName},
		},
		func() float64 {
			c := Collect(d, crc8Table)
			return float64(c.power)
		},
	)

	// 電圧の取得
	d.voltageDurations = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name:        "REXBTWATTCH_Voltage",
			Help:        "REX-BTWATTCH",
			ConstLabels: prometheus.Labels{"Name": d.DeviceName},
		},
		func() float64 {
			c := Collect(d, crc8Table)
			return float64(c.voltage)
		},
	)

	// 電流の取得
	d.ampereDurations = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name:        "REXBTWATTCH_Ampere",
			Help:        "REX-BTWATTCH",
			ConstLabels: prometheus.Labels{"Name": d.DeviceName},
		},
		func() float64 {
			c := Collect(d, crc8Table)
			log.Print(c)
			return float64(c.current)
		},
	)
}

// デバイスの終了処理
func (d *Device) finalizeDevice() {
	log.Printf("ポートを閉じました[" + d.DevicePath + "]")
	d.Port.Close()
}

var ()

// データの取得処理
func Collect(d *Device, crc8Table *crc8.Table) CollectionData {

	if !d.collectionDataLast.lastCollectDate.IsZero() {
		// 最終取得時間が現在からどれだけ前か確認する
		duration := time.Since(d.collectionDataLast.lastCollectDate)
		// 500ミリ秒以上していない場合
		if duration.Nanoseconds() < int64(500)*int64(time.Millisecond) {
			// 直前のデータを返す
			return d.collectionDataLast
		}
	}

	buf := make([]uint8, BUF_SIZE)

	if ret := request_measure(d.Port, buf, crc8Table); ret != 0 {
		log.Printf("計測エラー")
		return CollectionData{}
	}

	// 最終取得データとして登録する
	d.collectionDataLast = dataParse(buf)

	return d.collectionDataLast
}

/**
 * RTC タイマー設定コマンド作成関数
 * @param fyload 作成ペイロード格納用変数
 */
func create_timer_payload(payload []uint8) {
	/* 現在時刻を取得 */
	local := time.Now()
	year := local.Year()
	year %= 100

	/* 現在時刻を設定 */
	payload[0] = RTC_TMR_SND_CMD
	payload[1] = uint8(local.Second())
	payload[2] = uint8(local.Minute())
	payload[3] = uint8(local.Hour())
	payload[4] = uint8(local.Day())
	payload[5] = uint8(local.Month())
	payload[6] = uint8(year)
	payload[7] = uint8(local.Weekday())
}

/**
 * コマンド生成関数
 * @param cmd 作成コマンド格納用変数
 * @param payload ペイロード
 * @param payload_size ペイロード数
 */
func create_command(cmd []uint8, pld []uint8, pld_size int, crc8Table *crc8.Table) {
	cmd_size := pld_size - HDR_CNT_CRC_LENGTH

	/* ヘッダーを設定 */
	cmd[0] = HDR_CMD /* ヘッダー指定(固定値) */

	/* カウントを設定 */
	cmd[1] = uint8(cmd_size)               /* LoByte */
	cmd[2] = uint8((cmd_size >> 8) & 0xFF) /* HiByte */

	/* ペイロードの値を設定 */
	for i := 0; i < cmd_size; i++ {
		cmd[i+3] = pld[i]
	}

	/* CRCの値を設定 */
	cmd[cmd_size+3] = crc8.Update(0, pld[0:cmd_size], crc8Table)
}

/**
 * コマンド通信関数
 * @param buf 表示データ(リアルタイム計測データ転送要求コマンドの応答データ)
 */
func communicate_command(port serial.Port, wbuf []uint8, wcount int, rbuf []uint8, rcount int) int {

	if ret := xwrite(port, wbuf, wcount); ret < 0 {
		log.Printf("xwrite error")
		return -1
	}

	if ret := xread(port, rbuf, rcount); ret < 0 {
		log.Printf("xread error")
		return -1
	}

	if rbuf[4] != 0x00 {
		// fprintf(stderr, "received code error\n")
		log.Printf("eceived code error[" + strconv.Itoa(int(rbuf[4])) + "]")
		return -1
	}

	return 0
}

/**
 * write関数のwrapper
 */
func xwrite(port serial.Port, buf []uint8, count int) int {

	ret := -1

	for len := 0; len < count; len += ret {
		n, err := port.Write(buf[len:count])
		ret = n
		if err != nil || ret < 0 {
			// perror("write");
			log.Printf("書き込みエラー[" + err.Error() + "]")
			return 0
		}
	}

	return ret
}

/**
 * read関数のwrappwer
 */
func xread(port serial.Port, buf []uint8, count int) int {
	ret := -1

	for length := 0; length < count; length += ret {
		// 配列の長さ確認
		if length < 0 {
			log.Printf("読み込みエラー([)lenが0以下です)[length=" + strconv.Itoa(length) + "]")
			return 0
		}
		if length >= len(buf) {
			log.Printf("読み込みエラー([)lengthがbufより多いです)[length=" + strconv.Itoa(length) + "],[buf=" + strconv.Itoa(len(buf)) + "]")
			return 0
		}
		if count-length < 0 {
			log.Printf("読み込みエラー([)count-lengthが0以下です)[count-length=" + strconv.Itoa(length) + "]")
			return 0
		}
		if count-length >= len(buf) {
			log.Printf("読み込みエラー([)count-lengthがbufより多いです)[count-length=" + strconv.Itoa(length) + "],[buf=" + strconv.Itoa(len(buf)) + "]")
			return 0
		}
		n, err := port.Read(buf[length:count])
		ret = n
		// log.Printf(strconv.Itoa(int(count)) + "読み込み予定")
		// log.Printf(strconv.Itoa(int(ret)) + "バイト読み込み")
		if err != nil || ret < 0 {
			// perror("read")
			log.Printf("読み込みエラー[" + err.Error() + "]")
			return 0
		}
	}

	return ret
}

func init_wattchecker(port serial.Port, crc8Table *crc8.Table) {
	/* ペイロード:RTC タイマー設定コマンド */
	pld := make([]uint8, RTC_TMR_SND_LENGTH)
	buf := make([]uint8, BUF_SIZE)
	cmd := make([]uint8, RTC_TMR_SND_LENGTH)

	create_timer_payload(pld)
	create_command(cmd, pld, RTC_TMR_SND_LENGTH, crc8Table)
	communicate_command(port,
		cmd, RTC_TMR_SND_LENGTH,
		buf, RTC_TMR_RCV_LENGTH)
}

/**
 * ワットチェッカー計測開始関数
 * @param fd ファイルディスクリプタ
 */
func start_measure(port serial.Port, crc8Table *crc8.Table) int {
	log.Printf("計測開始中...")
	/* ペイロード:計測開始コマンド(0xFF:テスト用高速測定モード
	 *			  0x00:テスト用高速測定モード解除)
	 */
	pld := []uint8{SRT_MSR_SND_CMD, 0x00}
	buf := make([]uint8, BUF_SIZE)
	cmd := make([]uint8, SRT_MSR_SND_LENGTH)

	create_command(cmd, pld, SRT_MSR_SND_LENGTH, crc8Table)
	ret := communicate_command(port,
		cmd, SRT_MSR_SND_LENGTH,
		buf, SRT_MSR_RCV_LENGTH)
	log.Printf("完了")
	return ret
}

/**
 * ワットチェッカー計測データ転送要求関数
 * @param fd ファイルディスクリプタ
 */
func request_measure(port serial.Port, buf []uint8, crc8Table *crc8.Table) int {
	/* ペイロード:リアルタイム計測データ転送要求コマンド */
	pld := []uint8{RTM_MSR_SND_CMD}
	cmd := make([]uint8, RTM_MSR_SND_LENGTH)

	create_command(cmd, pld, RTM_MSR_SND_LENGTH, crc8Table)
	ret := communicate_command(port,
		cmd, RTM_MSR_SND_LENGTH,
		buf, RTC_MSR_RCV_LENGTH)
	return ret
}

// 収集データを構造体に置き換える
func dataParse(buf []uint8) CollectionData {
	collectionData := CollectionData{}

	// 取得データの解析を行う
	current := DATA(buf[7], buf[6], buf[5])
	voltage := DATA(buf[10], buf[9], buf[8])
	power := DATA(buf[13], buf[12], buf[11])

	// データの登録
	collectionData.lastCollectDate = time.Now()
	collectionData.time = time.Date(
		(2000 + int(buf[19])), // 年
		time.Month(buf[18]),   // 月
		int(buf[17]),          // 日
		int(buf[16]),          // 時
		int(buf[15]),          // 分
		int(buf[14]),          // 秒
		0,
		time.Now().Location())

	// 受信データのダンプ表示(デバ用)
	// for i := 0; i < len(buf); i++ {
	// 	log.Printff("%03d ", buf[i])
	// }

	collectionData.current = TO_MA(current)
	collectionData.voltage = TO_V(voltage)
	collectionData.power = TO_W(power)

	return collectionData
}

/**
 * 計測データ詳細表示関数
 * @param buf 表示データ(リアルタイム計測データ転送要求コマンドの応答データ)
 */
func disp_data_details(collectionData CollectionData) {

	log.Printf(collectionData.time)

	log.Printf("voltage = %3.2fV , current = %4.2fmA , power = %4.2fW\n",
		collectionData.voltage, collectionData.current, collectionData.power)
}

func DATA(a uint8, b uint8, c uint8) int32 {
	return ((int32(a) << 16) | (int32(b) << 8) | int32(c))
}

func TO_V(v int32) float32 {
	return (float32(v) * VOLTAGE_STEP / 1000) /* V */
}

func TO_MA(v int32) float32 {
	return (float32(v) * CURRENT_STEP) /* mA */
}

func TO_W(v int32) float32 {
	return (float32(v) * POWER_STEP / 1000) /* W */
}
