# cutlog
golang 高并发日志库 切割方式分为日期切割、文件大小切割和不切割，可自定义多文件切割<br />

用法类似日志工具包log4j<br />

**打印日志有5个方法 Println DebugPrintln，InfoPrintln，ErrorPrintln 日志级别等信息可在配置文件中自定义<br />

日志文件中  [Log] 为根配置，添加相应子节点（[Debug] [Error]）实现级别设定，设置_enable为false不输出文件，<br />
Is_debug=true表示输出日志的至控制台，<br />
CutType为切割类型：<br />
    &emsp;1：按日期切割（DatePattern）设置为go的time规范，按日期建立文件夹时设置DirDataPattern,设置为空不建文件夹<br />
    &emsp;2：按文件大小切割（MaxFileSize）。MaxFileSize设置文件切割大小<br />
    &emsp;3：不切割，直接输出日志到文件中。<br />
日志输出流程：<br />
    &emsp;1.启动日志服务 cutlog.StartProvider()<br />
        &emsp;&emsp;1.1 加载配置项数据SetConfig<br />
        &emsp;&emsp;1.2 配置日志文件路径建立相应文件夹<br />
        &emsp;&emsp;1.3 加载日志文件类型settingArr，将子节点信息推至yXPSetting的struct中<br />
        &emsp;&emsp;1.4 为每个自定义日志文件启动两个监控线程<br />
           &emsp;&emsp;&emsp;1.4.1 监控channel，消费channel数据并推至buffer中，判断是否需要切割文件或buffer超过设置，写文件<br />
           &emsp;&emsp;&emsp;1.4.2 启动timer，每5秒将buffer写入文件，防长时间不写入日志不同步问题<br />
    &emsp;2.调用 Println 将日志推至channel<br />
    &emsp;3.自定义相关方法对日志文件进行输出 如：DebugPrintln  ErrorPrintln InfoPrintln<br />


#修改

