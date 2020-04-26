#include "server.h"
#include <QNetworkDatagram>
#include <QtEndian>

void Server::initSocket()
{
    udpSocket = new QUdpSocket(this);
    udpSocket->bind(QHostAddress::Any, 45467);

    connect(udpSocket, &QUdpSocket::readyRead,
            this, &Server::readPendingDatagrams);

    memset(lead_count, 0, sizeof(lead_count));
    buffer_size = 4;
    buffer_health = -1;
    timerId = startTimer(500);
}

void Server::checkIfExists(uint8_t playerNumber, uint32_t count)
{
    if (!inputs[playerNumber].contains(count)) //They are asking for a value we don't have
    {
        if (!buttons[playerNumber].isEmpty())
            inputs[playerNumber][count] = buttons[playerNumber].takeFirst();
        else if (inputs[playerNumber].contains(count-1))
            inputs[playerNumber][count] = inputs[playerNumber][count-1];
        else
            inputs[playerNumber][count] = qMakePair(0, 0/*Controller not present*/);
    }
}

void Server::sendInput(uint32_t count, QHostAddress address, int port, uint8_t playerNum, uint8_t spectator)
{
    char buffer[512];
    uint32_t count_lag = lead_count[playerNum] - count;
    buffer[0] = 1; // Key info from server
    buffer[1] = playerNum;
    qToBigEndian(count_lag, &buffer[2]);
    buffer[6] = buffer_size; //count number
    uint32_t curr = 7;
    for (uint8_t i = 0; i < buffer[6]; ++i)
    {
        if (spectator == 0 || inputs[playerNum].contains(count))
        {
            qToBigEndian(count, &buffer[curr]);
            curr += 4;
            checkIfExists(playerNum, count);
            qToBigEndian(inputs[playerNum][count].first, &buffer[curr]);
            curr += 4;
            buffer[curr] = inputs[playerNum][count].second;
            curr += 1;
        }
        ++count;
    }

    if (curr > 7)
        udpSocket->writeDatagram(&buffer[0], curr, address, port);
}

void Server::sendRegResponse(uint8_t playerNumber, uint32_t reg_id, QHostAddress address, int port)
{
    char buffer[3];
    buffer[0] = 3;
    buffer[1] = playerNumber;

    if (!reg.contains(playerNumber))
    {
        reg[playerNumber] = reg_id;
        inputs[playerNumber][0] = qMakePair(0, 1/*PLUGIN_NONE*/);
        buffer[2] = 1;
    }
    else
    {
        if (reg[playerNumber] == reg_id)
            buffer[2] = 1;
        else
            buffer[2] = 0;
    }

    udpSocket->writeDatagram(&buffer[0], 3, address, port);
}

void Server::readPendingDatagrams()
{
    uint32_t keys, count, reg_id;
    uint8_t playerNum;
    while (udpSocket->hasPendingDatagrams())
    {
        QNetworkDatagram datagram = udpSocket->receiveDatagram();
        QByteArray incomingData = datagram.data();
        playerNum = incomingData.at(1);
        switch (incomingData.at(0))
        {
            case 0: // key info from client
                count = qFromBigEndian<uint32_t>(&incomingData.data()[2]);
                keys = qFromBigEndian<uint32_t>(&incomingData.data()[6]);
                if (buttons[playerNum].size() < 2)
                    buttons[playerNum].append(qMakePair(keys, incomingData.at(10)));
                break;
            case 2: // request for player input data
                count = qFromBigEndian<uint32_t>(&incomingData.data()[2]);
                if (count >= lead_count[playerNum])
                {
                    buffer_health = incomingData.data()[7];
                    lead_count[playerNum] = count;
                }
                sendInput(count, datagram.senderAddress(), datagram.senderPort(), playerNum, incomingData.at(6));
                break;
            case 4: // registration request
                reg_id = qFromBigEndian<uint32_t>(&incomingData.data()[2]);
                if (playerNum < 4)
                    sendRegResponse(playerNum, reg_id, datagram.senderAddress(), datagram.senderPort());
                break;
            default:
                printf("Unknown packet type %d\n", incomingData.at(0));
                break;
        }
    }
}

void Server::timerEvent(QTimerEvent *)
{
    if (buffer_health != -1)
    {
        if (buffer_health > 3 && buffer_size > 3)
            --buffer_size;
        else if (buffer_health < 3)
            ++buffer_size;
    }
}
