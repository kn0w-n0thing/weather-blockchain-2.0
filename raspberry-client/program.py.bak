import os
import sys
import time
import hashlib
from PyQt5.QtWidgets import *
from PyQt5.QtGui import *
from PyQt5.QtCore import *

import RPi.GPIO as GPIO
import time 

GPIO.setmode(GPIO.BCM) 

#GPIO pins:
#clear = 17  bad1
#cloudy = 27
#thunder = 22  target2
#rain = 5   bad2
#snow = 6   target1
#fog = 13

#condition = [17,27,22,5,6,13]
condition = [6,27,5,22,17,13]

for o in range(6):
    GPIO.setup(condition[o],GPIO.OUT)
    GPIO.output(condition[o],GPIO.LOW)

def neon_switch(i):
    GPIO.cleanup()
    GPIO.setmode(GPIO.BCM)
    for o in range(6):
        GPIO.setup(condition[o],GPIO.OUT)
        GPIO.output(condition[o],GPIO.LOW)

    for n in range(6):
        if n == i:
            GPIO.output(condition[i],GPIO.HIGH)
            print("number "+ str(n) +" is lighted up")
        elif n != i:
            GPIO.output(condition[n],GPIO.LOW)
            print("number "+ str(n) +" is off")

def readQSS(path):
    with open(path,'r',encoding='utf-8') as fr:
        return fr.read()

def readJsonDicts(path):
    jsonDicts=[]
    with open(path,"r",encoding='utf-8') as fr:
        jsonStrs=fr.readlines()
        for jsonStr in jsonStrs:
            jsonDict = eval(jsonStr)
            jsonDicts.append(jsonDict)
    return jsonDicts

def loadFonts(dirPath):
    for file in os.listdir(dirPath):
        filePath=os.path.join(dirPath,file)
        QFontDatabase.addApplicationFont(filePath)
        
def getFileMD5(path):
    with open(path, 'rb') as fp:
        data = fp.read()
    md5= hashlib.md5(data).hexdigest()
    return md5

class Main(QWidget):
    
    def __init__(self,projectDir):
        super().__init__()
        self.md5=''
        self.winId=0
        self.winIndex=0
        self.cityDict={'BJ':'北京'}
        self.sourceDict={'Mj':'墨迹天气','Op':'OpenWeather','MS':'AruzeWeather','Xz':'国家气象局','Ac':'AccuWeather'}
        self.projectDir=projectDir
        self.iconDir=os.path.join(self.projectDir,'Icon')
        self.dataPath=os.path.join(self.projectDir,'WeatherData.txt')
        self.qssStyle=readQSS(os.path.join(self.projectDir,'style.qss'))
        loadFonts(os.path.join(self.projectDir,'Font'))
        self.winnerTimer = QTimer()
        self.dataTimer = QTimer()
        self.initGUI()
        self.initUI()
        self.refreshData()
        self.runRefreshDataAnimation(1000)
    
    def getQLabel(self,text,name,size=None,alignment=Qt.AlignCenter):
        label=QLabel(text,self)
        label.setObjectName(name)
        label.setFrameShape(QFrame.Box)
        label.setFrameShadow(QFrame.Raised)
        if size is not None:
            label.setFixedSize(size[0],size[1])
        label.setWordWrap(True)
        if alignment is not None:
            label.setAlignment(alignment)
        return label
    
    def getWinnerLayout(self):
        layout=QGridLayout()
        layout.setSpacing(9)
        layout.setContentsMargins(0,0,0,0)
        pixWinner=QPixmap(os.path.join(self.iconDir,'winner.png'))
        for i in range(3):
            lblWinner=self.getQLabel('','Winner',[174,45])
            lblWinner.setPixmap(pixWinner)
            opacity = QGraphicsOpacityEffect()
            opacity.setOpacity(0)
            lblWinner.setGraphicsEffect(opacity)
            layout.addWidget(lblWinner,0,i)
        return layout
    
    def runWinnerAnimation(self,interval):
        addValue=1
        opacityValue=addValue
        def timeout():
            nonlocal addValue,opacityValue
            qwidget=self.winnerLayout.itemAt(self.winIndex).widget()
            opacity = QGraphicsOpacityEffect()
            opacity.setOpacity(opacityValue/100)
            qwidget.setGraphicsEffect(opacity)
            opacityValue += addValue
            if opacityValue % 100 ==0:
                addValue = -addValue
        self.winnerTimer.setInterval(interval)
        self.winnerTimer.timeout.connect(timeout)
        self.winnerTimer.start()
        
    def stopWinnerAnimation(self):
        self.winnerTimer.stop()
        qwidget=self.winnerLayout.itemAt(self.winIndex).widget()
        opacity = QGraphicsOpacityEffect()
        opacity.setOpacity(0)
        qwidget.setGraphicsEffect(opacity)
    
    def getSourceLayout(self):
        layout=QGridLayout()
        layout.setSpacing(9)
        layout.setContentsMargins(0,0,0,0)
        for i in range(3):
            lblSource=self.getQLabel('','TitleSource',[174,85])
            layout.addWidget(lblSource,0,i)
        return layout
    
    def getTextRotateImgHtml(self,text,index,angle):
        if index==0:
            imgPath=os.path.join(self.iconDir,'wd_l',str(360-angle)+'_wd_l.png')
        else:
            imgPath=os.path.join(self.iconDir,'wd_s',str(360-angle)+'_wd_s.png')
        html='<p>'+text+'<img src="'+imgPath+'" /></p>'
        return html
    
    def getCurrentWeatherPanel(self):
        layout=QGridLayout()
        lbl0=self.getQLabel('','Space',[174,35],Qt.AlignTop | Qt.AlignHCenter)
        lbl1=self.getQLabel('','CurrentWeather',[174,75],Qt.AlignTop | Qt.AlignHCenter)
        lbl2=self.getQLabel('','CurrentWeather',[174,55],Qt.AlignTop | Qt.AlignHCenter)
        lbl3=self.getQLabel('','CurrentWeather',[174,55],Qt.AlignTop | Qt.AlignHCenter)
        lbl4=self.getQLabel('','CurrentWeather',[174,55],Qt.AlignTop | Qt.AlignHCenter)
        lbl5=self.getQLabel('','CurrentWeather',[174,55],Qt.AlignTop | Qt.AlignHCenter)
        lbl6=self.getQLabel('','CurrentWeather',[174,65],Qt.AlignTop | Qt.AlignHCenter)
        layout.addWidget(lbl0,0,0)
        layout.addWidget(lbl1,1,0)
        layout.addWidget(lbl2,2,0)
        layout.addWidget(lbl3,3,0)
        layout.addWidget(lbl4,4,0)
        layout.addWidget(lbl5,5,0)
        layout.addWidget(lbl6,6,0)
        layout.setSpacing(15)
        layout.setContentsMargins(0,0,0,0)
        panel=QWidget()
        panel.setFixedSize(174,485)
        panel.setLayout(layout)
        return panel
    
    def getCurrentWeatherPanelLayout(self):
        layout=QGridLayout()
        layout.setSpacing(9)
        layout.setContentsMargins(0,0,0,0)
        panel1=self.getCurrentWeatherPanel()
        panel2=self.getCurrentWeatherPanel()
        panel3=self.getCurrentWeatherPanel()
        layout.addWidget(panel1,0,1)
        layout.addWidget(panel2,0,2)
        layout.addWidget(panel3,0,3)
        return layout
    
    def getPastWeatherContents(self,dtime,Condition,Temp,rTemp,Hum,wSpeed,wDir,Source):
        layout=QGridLayout()
        lbl1=self.getQLabel(time.strftime('%Y-%m-%d',time.localtime(dtime)),'PastYMD',[220,25],Qt.AlignLeft | Qt.AlignVCenter)
        lbl2=self.getQLabel(time.strftime('%H:%M',time.localtime(dtime)),'PastHM',[220,55],Qt.AlignLeft | Qt.AlignVCenter)
        lbl4=self.getQLabel(Condition,'PastWeather',[320,25],Qt.AlignLeft | Qt.AlignVCenter)
        lbl5=self.getQLabel(str(Temp)+'°  ('+str(rTemp)+'°)','PastWeather',[320,25],Qt.AlignLeft | Qt.AlignVCenter)
        lbl6=self.getQLabel(self.getTextRotateImgHtml(str(Hum)+'%&nbsp;&nbsp;'+str(wSpeed)+'m/s&nbsp;&nbsp;'+str(int(wDir)),1,int(wDir)),'PastWeather',[320,25],Qt.AlignLeft | Qt.AlignVCenter)
        lbl7=self.getQLabel(self.sourceDict[Source],'PastSource',[320,40],Qt.AlignLeft | Qt.AlignVCenter)
        layout.addWidget(lbl1,0,0,1,1)
        layout.addWidget(lbl2,1,0,2,1)
        layout.addWidget(lbl4,0,1,1,1)
        layout.addWidget(lbl5,1,1,1,1)
        layout.addWidget(lbl6,2,1,1,1)
        layout.addWidget(lbl7,3,1,1,1)
        layout.setVerticalSpacing(0)
        layout.setContentsMargins(30,20,30,20)
        pastWeatherContents=QWidget()
        pastWeatherContents.setObjectName('PastWeatherContents')
        pastWeatherContents.setFixedSize(540,160)
        pastWeatherContents.setLayout(layout)
        return pastWeatherContents
    
    def getPastWeatherPanel(self,dataDicts=[]):
        layout=QGridLayout()
        layout.setSpacing(10)
        layout.setContentsMargins(0,0,0,0)
        i=0
        for dataDict in dataDicts:
            pwc=self.getPastWeatherContents(dataDict['time'],dataDict['Condition'],dataDict['Temp'],
                                            dataDict['rTemp'],dataDict['Hum'],dataDict['wSpeed'],
                                            dataDict['wDir'],dataDict['Source'])
            layout.addWidget(pwc,i,0)
            i+=1
        panel = QWidget()
        panel.setObjectName('PastWeatherPanel')
        panel.setLayout(layout)
        return panel
    
    def runRefreshDataAnimation(self,interval):
        self.dataTimer.setInterval(interval)
        self.dataTimer.timeout.connect(self.refreshData)
        self.dataTimer.start()
        
    def stopRefreshDataAnimation(self):
        self.dataTimer.stop()    
    
    def initGUI(self):
        self.lblSpace0=self.getQLabel('','Space',[540,10])
        self.lblTitCity=self.getQLabel('','TitleCity',[540,350])
        self.lblTitYMD=self.getQLabel('','TitleYMS',[540,40])
        self.lblSpace2=self.getQLabel('','Space',[540,40])
        self.lblTitHM=self.getQLabel('','TitleHM',[540,150])
        self.lblSpace3=self.getQLabel('','Space',[540,35])
        self.winnerLayout=self.getWinnerLayout()
        self.sourceLayout=self.getSourceLayout()
        self.lblSpace4=self.getQLabel('','Space',[540,10])
        self.currentWeatherPanelLayout=self.getCurrentWeatherPanelLayout()
        self.lblSpace5=self.getQLabel('','Space',[540,20])
        self.pastWeatherScrollArea = QScrollArea(self)
        self.pastWeatherScrollArea.setVerticalScrollBarPolicy(Qt.ScrollBarAlwaysOff)
        self.pastWeatherScrollArea.setFixedSize(540,1139)
        pastWeatherPanel=self.getPastWeatherPanel()
        self.pastWeatherScrollArea.setWidget(pastWeatherPanel)
        self.qgl=QGridLayout()
        self.qgl.setSpacing(0)
        self.qgl.setContentsMargins(0,0,0,0)
        self.qgl.addWidget(self.lblSpace0,0,0)
        self.qgl.addWidget(self.lblTitCity,1,0)
        self.qgl.addWidget(self.lblTitYMD,2,0)
        self.qgl.addWidget(self.lblSpace2,3,0)
        self.qgl.addWidget(self.lblTitHM,4,0)
        self.qgl.addWidget(self.lblSpace3,5,0)
        self.qgl.addLayout(self.winnerLayout,6,0)
        self.qgl.addLayout(self.sourceLayout,7,0)
        self.qgl.addWidget(self.lblSpace4,8,0)
        self.qgl.addLayout(self.currentWeatherPanelLayout,9,0)
        self.qgl.addWidget(self.lblSpace5,10,0)
        self.qgl.addWidget(self.pastWeatherScrollArea,11,0)
        self.scrollAreaWidgetContents = QWidget()
        self.scrollAreaWidgetContents.setFixedSize(540,1929)
        self.scrollAreaWidgetContents.setLayout(self.qgl)
        #sa = QScrollArea(self)
        #sa.setVerticalScrollBarPolicy(Qt.ScrollBarAlwaysOff)
        #sa.setWidget(self.scrollAreaWidgetContents)
        vbox=QVBoxLayout()
        vbox.setContentsMargins(0,0,0,0)
        vbox.addWidget(self.scrollAreaWidgetContents)
        self.setLayout(vbox)
        self.setGeometry(0, 0, 540,1929)
        
    def refreshData(self):
        md5=getFileMD5(self.dataPath)
        if md5==self.md5:
            return
        self.md5=md5
        self.stopWinnerAnimation()
        jsonDicts=readJsonDicts(self.dataPath)
        curWeatherDict=jsonDicts[-1]
        self.lblTitCity.setText(self.cityDict[curWeatherDict['data'][0]['City']])
        self.lblTitYMD.setText(time.strftime(' %Y-%m-%d',time.localtime(curWeatherDict['time'])))
        self.lblTitHM.setText(time.strftime('%H %M',time.localtime(curWeatherDict['time'])))
        for i in range(len(curWeatherDict['data'])):
            if curWeatherDict['data'][i]['win']==1:
                self.winIndex=i
                self.winId=curWeatherDict['data'][i]['Id']
            if curWeatherDict['data'][i]['Source'] in ['Ac','MS','Op']:
                self.currentWeatherPanelLayout.itemAt(i).widget().setObjectName('CurrentWeatherTitleEN')
            else:
                self.currentWeatherPanelLayout.itemAt(i).widget().setObjectName('CurrentWeatherTitleCN')
            self.sourceLayout.itemAt(i).widget().setText(self.sourceDict[curWeatherDict['data'][i]['Source']])
            self.currentWeatherPanelLayout.itemAt(i).widget().layout().itemAt(1).widget().setText(curWeatherDict['data'][i]['Condition'])
            self.currentWeatherPanelLayout.itemAt(i).widget().layout().itemAt(2).widget().setText(str(curWeatherDict['data'][i]['Temp'])+'°')
            self.currentWeatherPanelLayout.itemAt(i).widget().layout().itemAt(3).widget().setText('('+str(curWeatherDict['data'][i]['rTemp'])+'°)')
            self.currentWeatherPanelLayout.itemAt(i).widget().layout().itemAt(4).widget().setText(str(curWeatherDict['data'][i]['Hum'])+'%')
            self.currentWeatherPanelLayout.itemAt(i).widget().layout().itemAt(5).widget().setText(str(curWeatherDict['data'][i]['wSpeed'])+'m/s')
            angle=int(curWeatherDict['data'][i]['wDir'])
            self.currentWeatherPanelLayout.itemAt(i).widget().layout().itemAt(6).widget().setText(self.getTextRotateImgHtml(str(angle),0,angle))
        self.scrollAreaWidgetContents.setObjectName('Contents'+str(self.winIndex))
        self.runWinnerAnimation(12) #奖杯闪烁速度
        dataDicts=[]
        for jsonDict in jsonDicts[::-1][1:]:
            for dataDict in jsonDict['data']:
                if dataDict['win']==1:
                    dataDict['time']=jsonDict['time']
                    dataDicts.append(dataDict)
        panel=self.getPastWeatherPanel(dataDicts)
        self.pastWeatherScrollArea.setWidget(panel)
        self.setStyleSheet(self.qssStyle)
        neon_switch(self.winId)
        
    def initUI(self):
        self.show()
        self.showFullScreen()
        self.setCursor(Qt.BlankCursor)  #隐藏鼠标
        
if __name__=='__main__':
    projectDir=r''
    app = QApplication(sys.argv)
    main=Main(projectDir)
    sys.exit(app.exec())
