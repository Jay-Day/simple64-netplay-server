FROM quay.io/centos/centos:stream8
RUN dnf -y update && \
    dnf install -y unzip wget qt5-qtwebsockets && \
    wget https://github.com/m64p/m64p-netplay-server/releases/latest/download/m64p-netplay-server-linux.zip && \
    unzip /m64p-netplay-server-linux.zip && \
    dnf remove -y unzip wget && \
    dnf clean all -y && \
    rm /m64p-netplay-server-linux.zip && \
    chmod +x /m64p-netplay-server
ENTRYPOINT ["/m64p-netplay-server"]
